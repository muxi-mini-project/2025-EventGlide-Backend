package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdkerrorx "github.com/muxi-Infra/auditor-Backend/sdk/v2/api/errorx"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/client"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ dao.AuditorRepository = (*dao.AuditorRepo)(nil)

// ErrFormAlreadyPushed 表示该表单已成功送审，后台轮询应跳过，避免重复送审。
var ErrFormAlreadyPushed = errors.New("auditor form already pushed")

// uploadClaimLease 上传占用的租约时长：须大于单次上传的最坏耗时，
// 以免上传进行中租约到期、被另一个 worker 重复上传；worker 崩溃后租约到期可自动重试。
const uploadClaimLease = 60 * time.Second

type AuditorService interface {
	UploadForm(c context.Context, aw *req.AuditWrapper, FormId int64) error
	CreateAuditorForm(c context.Context, ActId int64, FormUrl string, Sub string) (*model.AuditorForm, error)
	GetOrCreatePendingForm(c context.Context, ActId int64, FormUrl string, Sub string) (*model.AuditorForm, error)
	ClaimForUpload(c context.Context, FormId int64) (time.Time, bool, error)
	ReleaseClaim(c context.Context, FormId int64, leaseUntil time.Time) error
	MarkPushed(c context.Context, FormId int64, pushedAt time.Time) error
	ReconcilePendingForms(c context.Context)
	FindOrphanPostForms(c context.Context) ([]model.AuditorForm, error)
	RevokeForm(c context.Context, form model.AuditorForm) error
}

type auditorService struct {
	ApiKey      string
	HookUrl     string
	MuxiCli     *client.Client
	AuditorRepo dao.AuditorRepository

	l *zap.Logger

	// reconcileCursor 是入站对账的分批游标（表单 id）。仅对账 goroutine 读写，
	// 无并发访问；服务重启后归零从头扫，多实例各持一份会重复回查但幂等无害。
	reconcileCursor int64
}

// reconcileBatchSize 单批对账条数：用于限制每批 DB 读取与 HTTP 回查量，
// 配合游标在轮次间推进，避免一次拉全表。
const reconcileBatchSize = 100

func NewAuditorService(repo dao.AuditorRepository, cfg *config.Conf, l *logger.LoggerSet) AuditorService {
	muxiCli, err := client.NewClient(client.Config{
		ApiKey: cfg.Auditor.ApiKey,
		Region: cfg.Auditor.Region,
	})
	if err != nil {
		l.Auditor.Fatal("Failed to create Muxi Auditor client", zap.Error(err))
		panic(err)
	}

	c := &auditorService{
		ApiKey:      cfg.Auditor.ApiKey,
		HookUrl:     cfg.Auditor.HookURL,
		MuxiCli:     muxiCli,
		AuditorRepo: repo,
		l:           l.Auditor.Named("service"),
	}
	return c
}

func (a *auditorService) UploadForm(c context.Context, aw *req.AuditWrapper, id int64) error {
	uploadReq, err := converter.AuditorUploadReqFromWrapper(aw, id, a.HookUrl)
	if err != nil {
		a.l.Error("Build auditor upload req failed", zap.Error(err))
		return errs.ErrUploadFormFailed.Wrap(err)
	}
	// 平台可能以 HTTP 200 + 错误业务码返回失败，SDK 此时 err 为 nil，必须校验业务码，
	// 否则失败会被当作成功标记已推送，活动将永久停留在 pending_auditor。
	resp, err := a.MuxiCli.UploadItem(c, &uploadReq)
	if err != nil {
		a.l.Error("Upload to auditor failed", zap.Error(err), zap.Int64("formId", id))
		return errs.ErrUploadFormFailed.Wrap(err)
	}
	if resp.Basic.Code != sdkerrorx.SuccessCode {
		// 打印平台原始错误（Errorx 内含 "http request failed: status=... body=..."），
		// 便于定位非 2xx 的真实原因，而非只看到 SDK 兜底码。
		a.l.Error("Auditor upload not accepted",
			zap.Int("code", resp.Basic.Code),
			zap.String("msg", resp.Basic.Msg),
			zap.Error(resp.Basic.Errorx),
			zap.Int64("formId", id),
			zap.String("region", a.MuxiCli.Region),
		)
		// 平台可能因历史重复上传已存在该条目（返回"该条目已被创建"）。
		// 此时回查其真实状态并落库，视为已送达，避免被永久挡在重复创建上。
		if ok, syncErr := a.syncExistingFormStatus(c, id); syncErr != nil {
			a.l.Error("Reconcile existing auditor item failed", zap.Error(syncErr), zap.Int64("formId", id))
		} else if ok {
			a.l.Info("Auditor item already exists, status synced", zap.Int64("formId", id))
			return nil
		}
		err := fmt.Errorf("auditor rejected upload: code=%d msg=%s", resp.Basic.Code, resp.Basic.Msg)
		return errs.ErrUploadFormFailed.Wrap(err)
	}
	return nil
}

// syncExistingFormStatus 回查平台已有条目的状态并写回 auditor_form。
// 返回 true 表示平台确实存在该条目（调用方应视为已送达，不再重传）。
// 只查询单个 id，故只要返回了条目即视为存在——不比对 hook_id：
// 平台的 hook_id 是 int64 雪花值，SDK 反序列化到 interface{} 会变成 float64 丢精度，
// 用 id 相等判断会永不命中，从而误判为"不存在"。
// 平台状态经 StatusMapper 映射为内部状态；映射不到时仍视为已存在（止住重传），仅告警不落库。
func (a *auditorService) syncExistingFormStatus(c context.Context, id int64) (bool, error) {
	req, err := request.NewGetItemsStatusReq([]int{int(id)})
	if err != nil {
		return false, err
	}
	resp, err := a.MuxiCli.GetItems(c, req)
	if err != nil {
		return false, err
	}
	if resp.Basic.Code != sdkerrorx.SuccessCode {
		a.l.Warn("Auditor get items not success",
			zap.Int("code", resp.Basic.Code), zap.String("msg", resp.Basic.Msg),
			zap.Error(resp.Basic.Errorx), zap.Int64("formId", id))
		return false, nil
	}
	if len(resp.Items) == 0 {
		return false, nil
	}
	// 只查询单个非零 id，平台按 ids 过滤返回，故返回的条目必为所查项。
	mapped := tools.StatusMapper(resp.Items[0].Status)
	if mapped == "" {
		a.l.Warn("Auditor item exists but status unmapped", zap.String("status", resp.Items[0].Status), zap.Int64("formId", id))
		return true, nil
	}
	if err := a.AuditorRepo.UpdateIfPending(c, id, mapped); err != nil {
		return true, err
	}
	return true, nil
}

func (a *auditorService) CreateAuditorForm(c context.Context, ActId int64, FormUrl string, sub string) (*model.AuditorForm, error) {
	return a.AuditorRepo.Insert(c, ActId, FormUrl, sub)
}

// GetOrCreatePendingForm 返回该活动待推送的审核表单，保证同一 (活动, subject) 至多一条记录
// （DB 侧由唯一索引兜底）：
//   - 已存在未推送的 form：复用，供上次上传失败后重试。
//   - 已存在且已推送的 form：返回 ErrFormAlreadyPushed，调用方跳过，避免重复送审。
//   - 完全不存在：新建；并发下若撞唯一键，回读已存在的行按上述规则处理。
func (a *auditorService) GetOrCreatePendingForm(c context.Context, ActId int64, FormUrl string, sub string) (*model.AuditorForm, error) {
	form, err := a.AuditorRepo.FindByActivity(c, ActId, sub)
	if err == nil {
		return reuseOrSkip(form)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	created, err := a.AuditorRepo.Insert(c, ActId, FormUrl, sub)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			form, findErr := a.AuditorRepo.FindByActivity(c, ActId, sub)
			if findErr != nil {
				return nil, findErr
			}
			return reuseOrSkip(form)
		}
		return nil, err
	}
	return created, nil
}

// reuseOrSkip 复用一个未推送表单；已推送则返回 ErrFormAlreadyPushed 让轮询跳过。
func reuseOrSkip(form model.AuditorForm) (*model.AuditorForm, error) {
	if form.PushedAt != nil {
		return nil, ErrFormAlreadyPushed
	}
	return &form, nil
}

func (a *auditorService) MarkPushed(c context.Context, FormId int64, pushedAt time.Time) error {
	return a.AuditorRepo.MarkPushed(c, FormId, pushedAt)
}

// FindOrphanPostForms 返回帖子已不存在的审核表单（待撤销队列）。
func (a *auditorService) FindOrphanPostForms(c context.Context) ([]model.AuditorForm, error) {
	return a.AuditorRepo.FindOrphanPostForms(c)
}

// RevokeForm 撤销一条帖子审核表单：远端可能存在的条目调用 DeleteItem 删除，成功后删本地行。
// 远端失败时保留本地行，交由后台下一轮重试。
//
// 判据不用调用方传入的快照，而是按 id 回查最新状态：
//   - 行已被其它实例撤销 → 幂等成功；
//   - pushed_at 或 claimed_at 任一非空即视为"可能已上传到远端"（上传成功但标记 pushed_at
//     失败会保留 claimed_at），据此决定是否调远端；
//   - 二者皆空 = 从未占用，直接删本地行。
func (a *auditorService) RevokeForm(c context.Context, form model.AuditorForm) error {
	current, err := a.AuditorRepo.FindByID(c, form.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if current.PushedAt != nil || current.ClaimedAt != nil {
		req, err := request.NewDeleteReq(uint(current.Id))
		if err != nil {
			return errs.ErrUploadFormFailed.Wrap(err)
		}
		resp, err := a.MuxiCli.DeleteItem(c, req)
		if err != nil {
			a.l.Error("Revoke auditor form failed", zap.Error(err), zap.Int64("formId", current.Id))
			return err
		}
		if resp.Basic.Code != sdkerrorx.SuccessCode {
			a.l.Error("Auditor delete not accepted",
				zap.Int("code", resp.Basic.Code),
				zap.String("msg", resp.Basic.Msg),
				zap.Int64("formId", current.Id),
			)
			return fmt.Errorf("auditor rejected delete: code=%d msg=%s", resp.Basic.Code, resp.Basic.Msg)
		}
	}
	return a.AuditorRepo.Delete(c, current.Id)
}

// ClaimForUpload 上传前原子占用该表单的上传权；返回租约到期时间与是否获得占用
// （false 表示已被其它执行者持有且租约未过期）。
// 时间截断到秒：claimed_at 是 datetime(0)，若带纳秒则读回时被截断，
// 与 ReleaseClaim 的相等比较将永不匹配。
func (a *auditorService) ClaimForUpload(c context.Context, FormId int64) (time.Time, bool, error) {
	now := time.Now().Truncate(time.Second)
	leaseUntil := now.Add(uploadClaimLease)
	ok, err := a.AuditorRepo.ClaimForUpload(c, FormId, now, leaseUntil)
	return leaseUntil, ok, err
}

// ReleaseClaim 上传失败时释放占用，让后续 tick 可立即重试，不必等租约到期。
func (a *auditorService) ReleaseClaim(c context.Context, FormId int64, leaseUntil time.Time) error {
	return a.AuditorRepo.ReleaseClaim(c, FormId, leaseUntil)
}

// ReconcilePendingForms 回查"已推送但平台仍 pending"的表单，拉取平台结论并落库。
// 平台回调可能丢失，此前的实现只在"上传被拒"时回查，导致丢失回调的表单永久停在 pending。
// 对账只同步平台已有结论；若平台根本没有该条目（历史推送丢失），仅告警不自动重推——
// 自动重推有重复创建风险（参见历史 "该条目已被创建" 事故），属需产品单独决策的扩展项。
//
// 分批游标：按 id 升序每批取 reconcileBatchSize 条，处理后推进游标；取空或不足一批即回绕到 0。
// 这样无论集合里是否有"平台永不结论"的钉子，都能在若干轮内覆盖全体，且单轮读取量有界。
func (a *auditorService) ReconcilePendingForms(c context.Context) {
	for {
		if c.Err() != nil {
			a.l.Warn("Reconcile pending forms aborted: context done", zap.Int64("cursor", a.reconcileCursor))
			return
		}
		forms, err := a.AuditorRepo.FindPushedPending(c, a.reconcileCursor, reconcileBatchSize)
		if err != nil {
			a.l.Error("Reconcile pending forms: find failed", zap.Error(err))
			return
		}
		for _, form := range forms {
			if c.Err() != nil {
				a.l.Warn("Reconcile pending forms aborted: context done", zap.Int64("cursor", a.reconcileCursor))
				return
			}
			ok, err := a.syncExistingFormStatus(c, form.Id)
			if err != nil {
				a.l.Error("Reconcile pending form failed", zap.Error(err), zap.Int64("formId", form.Id))
			} else if !ok {
				a.l.Warn("Pushed auditor form missing on platform, not re-pushing",
					zap.Int64("formId", form.Id), zap.Int64("targetId", form.ActivityId), zap.String("subject", form.Subject))
			}
			// 逐条推进游标：即使本批中途因 ctx 到期中断，下轮也从此处继续，不丢进度。
			a.reconcileCursor = form.Id
		}
		if len(forms) < reconcileBatchSize {
			a.reconcileCursor = 0
			return
		}
	}
}
