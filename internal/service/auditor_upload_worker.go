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
		if err := w.auditorService.UploadForm(ctx, aw, form.Id); err != nil {
			w.logger.Auditor.Error("Failed to upload form", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
			continue
		}
		if err := w.auditorService.MarkPushed(ctx, form.Id, time.Now()); err != nil {
			w.logger.Auditor.Error("Failed to mark form pushed", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
			continue
		}
		w.logger.Auditor.Info("Successfully uploaded form to auditor", zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
	}
}

func (w *AuditorUploadWorker) Stop() {
	w.ticker.Stop()
}
