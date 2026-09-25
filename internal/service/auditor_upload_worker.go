package service

import (
	"context"
	"errors"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/safe"
	"go.uber.org/zap"
)

// pendingActivitySource / pendingPostSource 抽象待送审数据的读取，便于单测注入。
type pendingActivitySource interface {
	FindPendingAuditorActivities(ctx context.Context) ([]model.Activity, error)
}

type pendingPostSource interface {
	FindPendingAuditorPosts(ctx context.Context) ([]model.Post, error)
}

type AuditorUploadWorker struct {
	activityRepo    pendingActivitySource
	postRepo        pendingPostSource
	auditorService  AuditorService
	logger          *logger.LoggerSet
	ticker          *time.Ticker
	reconcileTicker *time.Ticker
}

func NewAuditorUploadWorker(activityRepo *repo.ActivityRepo, postRepo *repo.PostRepo, auditorService AuditorService, logger *logger.LoggerSet) *AuditorUploadWorker {
	w := &AuditorUploadWorker{
		activityRepo:    activityRepo,
		postRepo:        postRepo,
		auditorService:  auditorService,
		logger:          logger,
		ticker:          time.NewTicker(5 * time.Second),
		reconcileTicker: time.NewTicker(10 * time.Minute),
	}
	safe.Go(logger.Auditor, "auditor-worker.upload", w.run)
	safe.Go(logger.Auditor, "auditor-worker.reconcile", w.runReconcile)
	return w
}

func (w *AuditorUploadWorker) run() {
	for range w.ticker.C {
		// 活动与帖子各自独立超时：共用同一 budget 时，前者积压会把预算耗尽，
		// 导致后者整轮被饿死并每 tick 报错。
		// 每轮单独 recover：单条数据的 panic 不应让整个 worker 永久停摆。
		safe.Run(w.logger.Auditor, "auditor-worker.activities", func() {
			w.processWithTimeout(w.processPendingAuditorActivities)
		})
		safe.Run(w.logger.Auditor, "auditor-worker.posts", func() {
			w.processWithTimeout(w.processPendingAuditorPosts)
		})
	}
}

// runReconcile 独立于上传循环运行：对账是慢速、逐条 HTTP 的长任务，
// 放同一 select 里会让上传被推迟到整轮对账结束。
func (w *AuditorUploadWorker) runReconcile() {
	for range w.reconcileTicker.C {
		safe.Run(w.logger.Auditor, "auditor-worker.reconcile", w.reconcile)
	}
}

// reconcile 回查已推送但仍 pending 的表单，弥补平台回调丢失。
func (w *AuditorUploadWorker) reconcile() {
	w.processWithTimeoutFor(2*time.Minute, w.auditorService.ReconcilePendingForms)
}

func (w *AuditorUploadWorker) processWithTimeout(fn func(context.Context)) {
	w.processWithTimeoutFor(30*time.Second, fn)
}

func (w *AuditorUploadWorker) processWithTimeoutFor(d time.Duration, fn func(context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	fn(ctx)
}

func (w *AuditorUploadWorker) processPendingAuditorActivities(ctx context.Context) {
	acts, err := w.activityRepo.FindPendingAuditorActivities(ctx)
	if err != nil {
		w.logger.Auditor.Error("Failed to find pending auditor activities", zap.Error(err))
		return
	}

	for _, act := range acts {
		// 历史活动可能没有申请表（早期 activeForm 非必填）。空表单仍送审，
		// 但显式告警，避免与正常带表活动混在同一句成功日志里。
		if act.ActiveForm == "" {
			w.logger.Auditor.Warn("Activity has no active form, uploading without it", zap.Int64("actId", act.Id))
		}
		aw := &req.AuditWrapper{
			Subject:   SubjectActivity,
			StudentId: act.StudentID,
			CactReq:   converter.ActivityToAuditReq(&act),
		}
		w.uploadPendingForm(ctx, act.Id, act.ActiveForm, SubjectActivity, aw, zap.Int64("actId", act.Id))
	}
}

func (w *AuditorUploadWorker) processPendingAuditorPosts(ctx context.Context) {
	posts, err := w.postRepo.FindPendingAuditorPosts(ctx)
	if err != nil {
		w.logger.Auditor.Error("Failed to find pending auditor posts", zap.Error(err))
		return
	}

	for _, post := range posts {
		aw := &req.AuditWrapper{
			Subject:   SubjectPost,
			StudentId: post.StudentID,
			CpostReq:  converter.PostToAuditReq(&post),
		}
		w.uploadPendingForm(ctx, post.Id, "", SubjectPost, aw, zap.Int64("postId", post.Id))
	}
}

// uploadPendingForm 对单条待送审记录执行「取/建 form -> 占用租约 -> 上传 -> 标记」的幂等序列。
// idField 用于区分活动/帖子的日志字段（actId / postId）。
func (w *AuditorUploadWorker) uploadPendingForm(ctx context.Context, id int64, formUrl, subject string, aw *req.AuditWrapper, idField zap.Field) {
	// 复用尚未成功推送的 form；若该记录已送审成功则跳过，避免回调到达前每个 tick 重复送审。
	form, err := w.auditorService.GetOrCreatePendingForm(ctx, id, formUrl, subject)
	if err != nil {
		if errors.Is(err, ErrFormAlreadyPushed) {
			return
		}
		w.logger.Auditor.Error("Failed to get or create auditor form", zap.Error(err), idField)
		return
	}
	// 上传前原子占用，避免多个实例同时读到同一未推送表单而各自重复上传。
	leaseUntil, claimed, err := w.auditorService.ClaimForUpload(ctx, form.Id)
	if err != nil {
		w.logger.Auditor.Error("Failed to claim form for upload", zap.Error(err), idField, zap.Int64("formId", form.Id))
		return
	}
	if !claimed {
		return
	}
	if err := w.auditorService.UploadForm(ctx, aw, form.Id); err != nil {
		w.logger.Auditor.Error("Failed to upload form", zap.Error(err), idField, zap.Int64("formId", form.Id))
		w.releaseClaim(form.Id, idField, leaseUntil)
		return
	}
	// 上传已成功，标记本地状态失败不应触发远端重传：本地重试标记；仍失败则保留占用，
	// 由租约到期后再兜底（最坏为 at-least-once，不会每 5s 重传）。
	if err := w.markPushedWithRetry(ctx, form.Id); err != nil {
		w.logger.Auditor.Error("Failed to mark form pushed after retries", zap.Error(err), idField, zap.Int64("formId", form.Id))
		return
	}
	w.logger.Auditor.Info("Successfully uploaded form to auditor", idField, zap.Int64("formId", form.Id))
}

func (w *AuditorUploadWorker) releaseClaim(formId int64, idField zap.Field, leaseUntil time.Time) {
	// 用独立短超时 ctx：入参 ctx 可能已随本次 tick 超时被取消，否则释放会失败、占用只能等租约自然到期。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.auditorService.ReleaseClaim(ctx, formId, leaseUntil); err != nil {
		w.logger.Auditor.Error("Failed to release form claim", zap.Error(err), idField, zap.Int64("formId", formId))
	}
}

// markPushedWithRetry 本地重试标记送审成功，避免因一次 DB 抖动导致下个 tick 重新远端上传。
func (w *AuditorUploadWorker) markPushedWithRetry(ctx context.Context, formId int64) error {
	const attempts = 3
	var err error
	for i := 1; i <= attempts; i++ {
		err = w.auditorService.MarkPushed(ctx, formId, time.Now())
		if err == nil {
			return nil
		}
		if i == attempts || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(i) * 300 * time.Millisecond):
		}
	}
	return err
}

func (w *AuditorUploadWorker) Stop() {
	if w.ticker != nil {
		w.ticker.Stop()
	}
	if w.reconcileTicker != nil {
		w.reconcileTicker.Stop()
	}
}
