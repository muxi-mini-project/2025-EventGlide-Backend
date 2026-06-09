package service

import (
	"context"
	"time"

	"github.com/raiki02/EG/api/req"
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
		aw := &req.AuditWrapper{
			Subject:   SubjectActivity,
			StudentId: act.StudentID,
		}
		form, err := w.auditorService.CreateAuditorForm(ctx, act.Id, act.ActiveForm, SubjectActivity)
		if err != nil {
			w.logger.Auditor.Error("Failed to create auditor form", zap.Error(err), zap.Int64("actId", act.Id))
			continue
		}
		if err := w.auditorService.UploadForm(ctx, aw, form.Id); err != nil {
			w.logger.Auditor.Error("Failed to upload form", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
			continue
		}
		w.logger.Auditor.Info("Successfully uploaded form to auditor", zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
	}
}

func (w *AuditorUploadWorker) Stop() {
	w.ticker.Stop()
}
