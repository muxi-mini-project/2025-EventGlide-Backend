package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type CallbackAuditorService interface {
	UpdateStatus(c context.Context, id int64, status string, reason string) error
}

type callbackAuditorService struct {
	repo dao.AuditorRepository
	aud  AuditorService
	l    *zap.Logger
}

func (ad *callbackAuditorService) UpdateStatus(c context.Context, id int64, status string, reason string) error {
	mapped := tools.StatusMapper(status)
	if mapped == "" {
		return errs.ErrAuditorStatusInvalid.Wrap(errors.New("unknown auditor status: " + status))
	}

	// 回调没有顺序号/版本号，延迟到达的旧 pass 回调会把已驳回的活动错误放行。
	// 仅当本地是活动表单且已驳回时，先向平台确认当前结论，避免被过期回调覆盖。
	if mapped == model.StancePass {
		if form, err := ad.repo.FindByID(c, id); err == nil &&
			form.Subject == model.SubjectActivity && form.Status == model.StanceReject {
			ok, confirmErr := ad.aud.ConfirmPass(c, id)
			if confirmErr != nil {
				// 回查失败保持改动前的放行行为，不因平台抖动让回调无法落库。
				ad.l.Warn("confirm pass on platform failed, accept callback anyway",
					zap.Int64("formId", id), zap.Error(confirmErr))
			} else if !ok {
				ad.l.Warn("ignore delayed pass callback: platform status is not pass", zap.Int64("formId", id))
				return nil
			}
		}
	}

	if mapped == model.StanceReject {
		if runes := []rune(reason); len(runes) > model.AuditorReasonMaxLen {
			ad.l.Warn("auditor callback reason truncated",
				zap.Int64("id", id),
				zap.Int("reasonLen", len(runes)),
				zap.Bool("truncated", true),
			)
			reason = string(runes[:model.AuditorReasonMaxLen])
		}
	}

	return ad.repo.Update(c, id, mapped, reason)
}

func NewCallbackAuditor(repo dao.AuditorRepository, aud AuditorService, l *logger.LoggerSet) CallbackAuditorService {
	return &callbackAuditorService{
		repo: repo,
		aud:  aud,
		l:    l.Auditor.Named("service"),
	}
}
