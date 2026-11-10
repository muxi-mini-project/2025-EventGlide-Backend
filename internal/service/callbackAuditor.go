package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// callbackConfirmTimeout 回查平台当前状态的超时：回调处理是同步的，
// 平台慢响应不能长时间占住连接，超时按"无法确认"处理（退回 pending 交给对账）。
const callbackConfirmTimeout = 3 * time.Second

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

	form, err := ad.repo.FindByID(c, id)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 回调查不到本地表单行：hook_id 与本地行对不上、或表单已被清理，返回成功避免平台无休止重试。
		return nil
	case err != nil:
		// 读不到本地状态就无法做回查确认：返回 error 让平台重试，连续失败最终交给对账兜底。
		ad.l.Warn("find auditor form failed, return error for retry",
			zap.Int64("formId", id), zap.Error(err))
		return err
	}

	// 帖子已不审核，其表单也不在对账范围内，保持直接写库；活动回调则必须先向平台确认。
	if form.Subject == model.SubjectActivity {
		// 回调没有顺序号/版本号：延迟到达的旧 pass/reject 会盖掉新一轮结论，
		// 平台重置为未审核的 pending 也可能已被后续结论取代。落结论前回查平台当前状态，
		// 以平台为权威源决定写什么。此路径非原子：FindByID -> 回查 -> 写库之间没有行锁，
		// 极端并发下仍可能互相覆盖；本改动只解决延迟到达的旧回调，不承诺并发原子性。
		ctx, cancel := context.WithTimeout(c, callbackConfirmTimeout)
		platformStatus, confirmErr := ad.aud.PlatformStatus(ctx, id)
		cancel()

		switch {
		case confirmErr != nil || platformStatus == "":
			// 无法确认平台结论。重置回调不能退回 pending：park 成 pending 后对账的
			// UpdateIfPending 读到 pending == pending 会早退，表单再也推不动，
			// 已出结论的活动也就永远退不回审核中。改为返回 error 让平台重试。
			if mapped == model.StancePending {
				ad.l.Warn("cannot confirm platform status for reset callback, return error for retry",
					zap.Int64("formId", id),
					zap.String("platformStatus", platformStatus),
					zap.Error(confirmErr))
				if confirmErr != nil {
					return confirmErr
				}
				return fmt.Errorf("cannot confirm platform status for reset callback: formId=%d", id)
			}
			// 终态回调无法确认时不应用本次回调：退回 pending，由 10 分钟一次的对账
			// 在平台给出结论后收敛（对账只扫 pending）。
			ad.l.Warn("cannot confirm platform status, park form as pending",
				zap.Int64("formId", id),
				zap.String("callbackStatus", mapped),
				zap.String("platformStatus", platformStatus),
				zap.Error(confirmErr))
			return ad.repo.ParkAsPending(c, id)

		case platformStatus == model.StancePending:
			// 平台还没有结论。此时应用终态可能是读延迟造成的误判，会把已发布活动短暂拉下来；
			// 退回 pending 同样会扰动，两者都不如忽略——回调丢失由对账兜底。
			if mapped != model.StancePending {
				ad.l.Warn("ignore terminal callback while platform still pending",
					zap.Int64("formId", id),
					zap.String("callbackStatus", mapped))
				return nil
			}
			// 重置回调与平台一致，落到下方正常写库，由 AfterUpdate 把活动退回 pending_auditor。

		default:
			// 平台已有结论。本地与之一致说明回调是旧的或重复的，忽略以免抖动、
			// 也避免驳回原因被短暂清空；不一致时以平台为准，只有回调与平台一致才带上回调里的原因。
			if form.Status == platformStatus {
				ad.l.Info("ignore callback already consistent with platform status",
					zap.Int64("formId", id), zap.String("status", platformStatus))
				return nil
			}
			if mapped != platformStatus {
				ad.l.Warn("callback conflicts with platform status, align to platform",
					zap.Int64("formId", id),
					zap.String("callbackStatus", mapped),
					zap.String("platformStatus", platformStatus))
				return ad.repo.Update(c, id, platformStatus, "")
			}
			ad.l.Info("auditor callback confirmed by platform status",
				zap.Int64("formId", id), zap.String("status", mapped))
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
