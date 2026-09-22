package service

import (
	"context"
	"errors"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

type AuditorUploadWorker struct {
	activityRepo   *repo.ActivityRepo
	auditorService AuditorService
	logger         *logger.LoggerSet
	ticker         *time.Ticker
}

func NewAuditorUploadWorker(activityRepo *repo.ActivityRepo, auditorService AuditorService, logger *logger.LoggerSet) *AuditorUploadWorker {
	w := &AuditorUploadWorker{
		activityRepo:   activityRepo,
		auditorService: auditorService,
		logger:         logger,
		ticker:         time.NewTicker(5 * time.Second),
	}
	go w.run()
	return w
}

func (w *AuditorUploadWorker) run() {
	for range w.ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		w.processPendingAuditorActivities(ctx)
		cancel()
	}
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
		// 复用尚未成功推送的 form；若该活动已送审成功则跳过，避免回调到达前每个 tick 重复送审。
		form, err := w.auditorService.GetOrCreatePendingForm(ctx, act.Id, act.ActiveForm, SubjectActivity)
		if err != nil {
			if errors.Is(err, ErrFormAlreadyPushed) {
				continue
			}
			w.logger.Auditor.Error("Failed to get or create auditor form", zap.Error(err), zap.Int64("actId", act.Id))
			continue
		}
		// 上传前原子占用，避免多个实例同时读到同一未推送表单而各自重复上传。
		leaseUntil, claimed, err := w.auditorService.ClaimForUpload(ctx, form.Id)
		if err != nil {
			w.logger.Auditor.Error("Failed to claim form for upload", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
			continue
		}
		if !claimed {
			continue
		}
		if err := w.auditorService.UploadForm(ctx, aw, form.Id); err != nil {
			w.logger.Auditor.Error("Failed to upload form", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
			w.releaseClaim(act.Id, form.Id, leaseUntil)
			continue
		}
		// 上传已成功，标记本地状态失败不应触发远端重传：本地重试标记；仍失败则保留占用，
		// 由租约到期后再兜底（最坏为 at-least-once，不会每 5s 重传）。
		if err := w.markPushedWithRetry(ctx, form.Id); err != nil {
			w.logger.Auditor.Error("Failed to mark form pushed after retries", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
			continue
		}
		w.logger.Auditor.Info("Successfully uploaded form to auditor", zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
	}
}

func (w *AuditorUploadWorker) releaseClaim(actId, formId int64, leaseUntil time.Time) {
	// 用独立短超时 ctx：入参 ctx 可能已随本次 tick 超时被取消，否则释放会失败、占用只能等租约自然到期。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.auditorService.ReleaseClaim(ctx, formId, leaseUntil); err != nil {
		w.logger.Auditor.Error("Failed to release form claim", zap.Error(err), zap.Int64("actId", actId), zap.Int64("formId", formId))
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
	w.ticker.Stop()
}
