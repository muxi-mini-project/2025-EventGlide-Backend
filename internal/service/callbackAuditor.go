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
	"gorm.io/gorm"
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
	// 仅当本地是活动表单且已驳回时，先向平台回查当前结论：
	// 只有平台明确 reject 才忽略这条 pass；pending/未知/查询失败都放行。
	// 若把 pending 也一并丢弃，本地会停在 reject，而对账只扫 pending，活动将无法恢复。
	if mapped == model.StancePass {
		form, err := ad.repo.FindByID(c, id)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 行不存在时交给 repo.Update 幂等处理，它查不到行会直接返回成功。
		case err != nil:
			ad.l.Warn("find auditor form failed, accept callback anyway",
				zap.Int64("formId", id), zap.Error(err))
		case form.Subject == model.SubjectActivity && form.Status == model.StanceReject:
			// 此路径非原子：FindByID -> 平台回查 -> repo.Update 之间没有行锁，
			// 极端并发下仍可能被后到的回调覆盖；本改动只解决延迟到达的旧回调，
			// 不承诺并发原子性。
			platformStatus, confirmErr := ad.aud.PlatformStatus(c, id)
			if confirmErr != nil {
				// 回查失败保持改动前的放行行为，不因平台抖动让回调无法落库。
				ad.l.Warn("get platform status failed, accept callback anyway",
					zap.Int64("formId", id), zap.Error(confirmErr))
			} else if platformStatus == model.StanceReject {
				ad.l.Warn("ignore delayed pass callback: platform status is reject",
					zap.Int64("formId", id), zap.String("platformStatus", platformStatus))
				return nil
			} else {
				ad.l.Info("accept pass callback: platform status is not reject",
					zap.Int64("formId", id), zap.String("platformStatus", platformStatus))
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
