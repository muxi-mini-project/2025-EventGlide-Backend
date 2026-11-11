package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/safe"
	"go.uber.org/zap"
)

// pendingActivitySource 抽象待送审数据的读取，便于单测注入。
type pendingActivitySource interface {
	FindPendingAuditorActivities(ctx context.Context) ([]model.Activity, error)
}

// perRecordUploadTimeout 单条记录送审序列（取/建 form -> 占用租约 -> 上传 -> 标记）的超时预算。
// 每条记录独立成 ctx：上传可能挂死，不能与整轮 tick 共享 deadline，否则同 tick 后续记录的
// DB 操作会复用一个已过期的 ctx（历史事故：一批 "Failed to get or create auditor form: context deadline exceeded"）。
const perRecordUploadTimeout = 30 * time.Second

var (
	auditorUploadInterval    = 5 * time.Second
	auditorReconcileInterval = 10 * time.Minute
	// auditorRevokeInterval 过期条目撤销是低频运维动作（活动过期后才需要），一小时一轮足够。
	auditorRevokeInterval = time.Hour
	// auditorStopTimeout 直接调用 Stop 时的兜底等待上限；Server.Shutdown 的总预算更小，会先截断。
	// 对账轮最长 2 分钟，超时即取消：对账幂等，下一轮启动后重跑。
	auditorStopTimeout = 10 * time.Second
)

type AuditorUploadWorker struct {
	activityRepo   pendingActivitySource
	auditorService AuditorService
	logger         *logger.LoggerSet

	mu      sync.Mutex
	baseCtx context.Context    // Start 传入的 ctx，单条记录从它派生（Stop 可取消在途请求）
	cancel  context.CancelFunc // 取消上传与对账两个循环
	started bool
	stopped bool
	wg      sync.WaitGroup
}

func NewAuditorUploadWorker(activityRepo *repo.ActivityRepo, auditorService AuditorService, logger *logger.LoggerSet) *AuditorUploadWorker {
	return &AuditorUploadWorker{
		activityRepo:   activityRepo,
		auditorService: auditorService,
		logger:         logger,
	}
}

// Start 启动上传与对账两个循环（幂等）。构造只注入依赖，不产生后台 goroutine。
func (w *AuditorUploadWorker) Start(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started || w.stopped {
		return
	}
	w.started = true
	runCtx, cancel := context.WithCancel(ctx)
	w.baseCtx = runCtx
	w.cancel = cancel
	w.wg.Add(3)
	safe.Go(w.logger.Auditor, "auditor-worker.upload", func() {
		defer w.wg.Done()
		w.run(runCtx)
	})
	safe.Go(w.logger.Auditor, "auditor-worker.reconcile", func() {
		defer w.wg.Done()
		w.runReconcile(runCtx)
	})
	safe.Go(w.logger.Auditor, "auditor-worker.revoke", func() {
		defer w.wg.Done()
		w.runRevoke(runCtx)
	})
}

// Stop 停止两个循环：取消在途请求并有界等待退出；幂等。
func (w *AuditorUploadWorker) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	cancel := w.cancel
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(auditorStopTimeout):
		w.logger.Auditor.Warn("auditor worker 停止超时", zap.Duration("timeout", auditorStopTimeout))
	}
}

func (w *AuditorUploadWorker) run(ctx context.Context) {
	ticker := time.NewTicker(auditorUploadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Stop 取消后 ctx.Done 与已触发的 ticker 可能同时就绪，这里再确认一次，
			// 避免关闭期间又跑一轮上传。
			if ctx.Err() != nil {
				return
			}
			// 活动与帖子各自独立超时：共用同一 budget 时，前者积压会把预算耗尽，
			// 导致后者整轮被饿死并每 tick 报错。
			// 每轮单独 recover：单条数据的 panic 不应让整个 worker 永久停摆。
			safe.Run(w.logger.Auditor, "auditor-worker.activities", func() {
				w.processWithTimeout(ctx, w.processPendingAuditorActivities)
			})
		}
	}
}

// runReconcile 独立于上传循环运行：对账是慢速、逐条 HTTP 的长任务，
// 放同一 select 里会让上传被推迟到整轮对账结束。
func (w *AuditorUploadWorker) runReconcile(ctx context.Context) {
	ticker := time.NewTicker(auditorReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Stop 取消后 ctx.Done 与已触发的 ticker 可能同时就绪，这里再确认一次。
			if ctx.Err() != nil {
				return
			}
			safe.Run(w.logger.Auditor, "auditor-worker.reconcile", func() {
				w.processWithTimeoutFor(ctx, 2*time.Minute, w.reconcile)
			})
		}
	}
}

// reconcile 回查已推送但仍 pending 的表单，弥补平台回调丢失。
func (w *AuditorUploadWorker) reconcile(ctx context.Context) {
	w.auditorService.ReconcilePendingForms(ctx)
}

// runRevoke 撤销所属活动已过期、平台仍 pending 的审核条目。
// 低频运维动作，一小时一轮；与上传/对账各自独立，互不拖延。
func (w *AuditorUploadWorker) runRevoke(ctx context.Context) {
	ticker := time.NewTicker(auditorRevokeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			safe.Run(w.logger.Auditor, "auditor-worker.revoke", func() {
				w.revoke(ctx)
			})
		}
	}
}

// revoke 整轮超时给到 5 分钟：单批 revokeBatchSize 条、条间有限流，
// 既要覆盖一批慢请求，又不能无限期占用 goroutine。
func (w *AuditorUploadWorker) revoke(ctx context.Context) {
	w.processWithTimeoutFor(ctx, 5*time.Minute, w.auditorService.RevokeExpiredForms)
}

// processWithTimeout 从 worker ctx 派生整轮预算：Stop 时整轮可取消，
// 单条记录再各自派生独立超时（见 recordCtx）。
func (w *AuditorUploadWorker) processWithTimeout(ctx context.Context, fn func(context.Context)) {
	w.processWithTimeoutFor(ctx, 30*time.Second, fn)
}

func (w *AuditorUploadWorker) processWithTimeoutFor(ctx context.Context, d time.Duration, fn func(context.Context)) {
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	fn(cctx)
}

// recordCtx 单条记录独立超时，但与 worker ctx 关联：Stop 能取消在途请求，
// 同时不共享整轮 tick 的 deadline。
func (w *AuditorUploadWorker) recordCtx(d time.Duration) (context.Context, context.CancelFunc) {
	w.mu.Lock()
	base := w.baseCtx
	w.mu.Unlock()
	if base == nil {
		base = context.Background()
	}
	return context.WithTimeout(base, d)
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
		w.uploadPendingForm(act.Id, act.ActiveForm, SubjectActivity, aw, zap.Int64("actId", act.Id))
	}
}

// uploadPendingForm 对单条待送审记录执行「取/建 form -> 占用租约 -> 上传 -> 标记」的幂等序列。
// idField 用于区分活动/帖子的日志字段（actId / postId）。
func (w *AuditorUploadWorker) uploadPendingForm(id int64, formUrl, subject string, aw *req.AuditWrapper, idField zap.Field) {
	// 每条记录独立超时，与整轮 tick 的 ctx 解耦：单条挂死的上传不应拖垮同 tick 后续记录。
	ctx, cancel := w.recordCtx(perRecordUploadTimeout)
	defer cancel()

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
	if err := w.markPushedWithRetry(form.Id); err != nil {
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
// 用独立短超时 ctx：此时上传已成功，标记不应因上传消耗了本条记录的 ctx 预算而失败。
func (w *AuditorUploadWorker) markPushedWithRetry(formId int64) error {
	const attempts = 3
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
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
