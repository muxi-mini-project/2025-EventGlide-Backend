package service

import (
	"context"
	"errors"
	"time"

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
	// 退回待审后平台还可能重投同一条 pass（try 字段不是递增序号，无法据此识别），
	// 若只看本地 reject 就会走"pending 直接放行"的快路径、再次绕过确认。
	// 因此只要本地尚未通过（pending/reject）就向平台回查当前结论：
	//   - 平台明确 pass：照常放行；
	//   - 平台明确 reject：忽略这条回调，保持本地现状；
	//   - pending/未知/回查失败：无法确认结论，退回待审（fail-closed）。
	// pass 不可回退：reject 分支只接受 pending_auditor 的活动，一旦误放行就无法
	// 再由回调纠正，所以无法确认时宁可退回 pending，交给 10 分钟一次的对账在
	// 平台给出结论后再决定；对账只扫 status='pending'，退回后正好进入其范围。
	// 已经是 pass 的表单不再回查，直接走下方原有写库逻辑（幂等）。
	if mapped == model.StancePass {
		form, err := ad.repo.FindByID(c, id)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 行不存在时交给 repo.Update 幂等处理，它查不到行会直接返回成功。
		case err != nil:
			// 读不到本地状态就无法判断是否需要回查：返回 error 让平台重试，
			// 连续失败最终交给对账兜底，不要再放行。
			ad.l.Warn("find auditor form failed, return error for retry",
				zap.Int64("formId", id), zap.Error(err))
			return err
		case form.Subject == model.SubjectActivity && form.Status != model.StancePass:
			// 此路径非原子：FindByID -> 平台回查 -> 条件更新之间没有行锁，
			// 极端并发下仍可能被后到的回调覆盖；本改动只解决延迟到达的旧回调，
			// 不承诺并发原子性。
			ctx, cancel := context.WithTimeout(c, 3*time.Second)
			defer cancel()
			platformStatus, confirmErr := ad.aud.PlatformStatus(ctx, id)
			switch {
			case confirmErr == nil && platformStatus == model.StancePass:
				ad.l.Info("accept pass callback: platform status confirmed pass",
					zap.Int64("formId", id), zap.String("platformStatus", platformStatus))
			case confirmErr == nil && platformStatus == model.StanceReject:
				ad.l.Warn("ignore delayed pass callback: platform status is reject",
					zap.Int64("formId", id), zap.String("platformStatus", platformStatus))
				return nil
			default:
				ad.l.Warn("cannot confirm platform status, revert form to pending",
					zap.Int64("formId", id),
					zap.String("platformStatus", platformStatus),
					zap.Error(confirmErr))
				return ad.repo.RevertRejectToPending(c, id)
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
