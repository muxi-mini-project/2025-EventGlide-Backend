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

// ErrAuditorItemNotFound 表示平台侧查不到该条目（按 id 查询返回空集）。
// 平台条目被撤销/从未创建成功时都会落到这里：对账用它区分"条目缺失"与一般查询失败，
// 撤销流程把它当作"已无需撤销"。
var ErrAuditorItemNotFound = errors.New("auditor item not found on platform")

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
	RevokeExpiredForms(c context.Context)
	GetReasons(c context.Context, activityIds []int64) (map[int64]string, error)
	PlatformStatus(c context.Context, formId int64) (string, error)
	DeleteItem(c context.Context, formId int64) error
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

	// revokeCursor 是过期条目撤销的分批游标（表单 id）。仅撤销 goroutine 读写，
	// 无并发访问；撤销结果持久化在 auditor_form.revoked_at 上（本地行保留作记录），
	// 因此扫描条件带 revoked_at IS NULL，处理完的行不会再回到游标范围内。
	revokeCursor int64
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
	if len(resp.Items) != 1 {
		// 无法确认哪一条对应该表单：不采信、不写库，但按"平台存在"处理以止住重传，
		// 避免条目数异常时被反复当成"不存在"重新推送。
		a.l.Warn("Auditor get items returned unexpected count, treated as existing",
			zap.Int("count", len(resp.Items)), zap.Int64("formId", id))
		return true, nil
	}
	// 只查询单个非零 id，平台按 ids 过滤返回，故恰好一条时即为所查项。
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

// GetReasons 批量返回活动的审核原因（activity_id -> reason）。
func (a *auditorService) GetReasons(c context.Context, activityIds []int64) (map[int64]string, error) {
	return a.AuditorRepo.FindReasonsByActivityIDs(c, activityIds)
}

// PlatformStatus 回查平台条目的审核状态，返回映射后的 pass/reject/pending；
// 平台状态无法识别时返回空串（未知），返回条目数不为 1 或查询失败返回 error。
// 回调无顺序号，延迟到达的旧 pass 回调可能把已驳回的活动错误放行，放行前据此确认当前结论。
func (a *auditorService) PlatformStatus(c context.Context, formId int64) (string, error) {
	req, err := request.NewGetItemsStatusReq([]int{int(formId)})
	if err != nil {
		return "", err
	}
	resp, err := a.MuxiCli.GetItems(c, req)
	if err != nil {
		return "", err
	}
	if resp.Basic.Code != sdkerrorx.SuccessCode {
		return "", fmt.Errorf("auditor get items not success: code=%d msg=%s", resp.Basic.Code, resp.Basic.Msg)
	}
	if len(resp.Items) == 0 {
		// 平台上没有该条目（已被撤销、或历史推送实际未成功）：返回可判定的哨兵错误，
		// 调用方据此区分"条目缺失"与一般查询失败。
		return "", fmt.Errorf("auditor item %d missing on platform: %w", formId, ErrAuditorItemNotFound)
	}
	if len(resp.Items) != 1 {
		// 仅当恰好返回一条时才采信：条目数异常时无法确认哪一条对应本次回查，
		// 取错条目会得出错误结论，因此一律报错交由调用方 fail-closed 处理。
		return "", fmt.Errorf("auditor item count unexpected: formId=%d count=%d", formId, len(resp.Items))
	}
	return tools.StatusMapper(resp.Items[0].Status), nil
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

// ReconcilePendingForms 回查"已推送但平台仍 pending"的活动表单，拉取平台结论并落库。
// 帖子表单已在 FindPushedPending 中按 subject 排除（帖子不审核），不在对账范围内。
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

// revokeBatchSize/revokeInterval 控制过期条目撤销的节奏：每批读取与平台请求量有界，
// 条间留出间隔，避免一轮把平台接口打满。
const (
	revokeBatchSize = 50
	revokeInterval  = 100 * time.Millisecond
)

// DeleteItem 撤销平台上的一条审核条目。条目是否真的消失由调用方回查确认：
// SDK 会把非 2xx 响应归一成业务码（拿不到原始状态码），单凭返回值不足以判定。
func (a *auditorService) DeleteItem(c context.Context, formId int64) error {
	req, err := request.NewDeleteReq(uint(formId))
	if err != nil {
		return err
	}
	resp, err := a.MuxiCli.DeleteItem(c, req)
	if err != nil {
		return err
	}
	// 平台可能以 HTTP 200 + 错误业务码返回失败，此时 err 为 nil，必须校验业务码。
	if resp.Basic.Code != sdkerrorx.SuccessCode {
		return fmt.Errorf("auditor delete not accepted: code=%d msg=%s", resp.Basic.Code, resp.Basic.Msg)
	}
	return nil
}

// RevokeExpiredForms 撤销"已送进平台、但所属活动已过期且平台仍 pending"的审核条目。
// 过期活动在送审与对账中都被跳过，但已经推进平台的条目会一直挂在审核队列，
// 由本任务撤销；撤销后保留本地表单行，只把撤销时间写进 revoked_at，扫描条件据此跳过它。
//
// 每条先回查平台状态：仍为 pending 才撤销，避免与刚完成的审核竞争；
// 平台已出结论的不撤销，直接标记完成（过期活动不再需要本地结论）。失败只记日志，下一轮重试。
// 分批游标与对账同构：按 id 升序每批 revokeBatchSize 条，取空或不足一批即回绕。
func (a *auditorService) RevokeExpiredForms(c context.Context) {
	for {
		if c.Err() != nil {
			a.l.Warn("Revoke expired auditor forms aborted: context done", zap.Int64("cursor", a.revokeCursor))
			return
		}
		forms, err := a.AuditorRepo.FindExpiredPushedPending(c, a.revokeCursor, revokeBatchSize)
		if err != nil {
			a.l.Error("Revoke expired auditor forms: find failed", zap.Error(err))
			return
		}
		for i, form := range forms {
			if c.Err() != nil {
				a.l.Warn("Revoke expired auditor forms aborted: context done", zap.Int64("cursor", a.revokeCursor))
				return
			}
			a.revokeForm(c, form)
			// 逐条推进游标：中途 ctx 到期也不丢进度。
			a.revokeCursor = form.Id
			if i < len(forms)-1 {
				select {
				case <-c.Done():
					return
				case <-time.After(revokeInterval):
				}
			}
		}
		if len(forms) < revokeBatchSize {
			a.revokeCursor = 0
			return
		}
	}
}

// markRevoked 持久化撤销结果。写库失败只记日志：下一轮扫描会再取到该行，
// 若平台条目确实已被撤销，回查会返回"条目缺失"并再次走到这里补写。
func (a *auditorService) markRevoked(c context.Context, form model.AuditorForm) {
	if err := a.AuditorRepo.MarkRevoked(c, form.Id, time.Now()); err != nil {
		a.l.Error("Mark expired auditor form revoked failed, will retry next round",
			zap.Error(err), zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId))
		return
	}
	a.l.Info("Revoked expired auditor form",
		zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId))
}

// revokeForm 对单条表单执行"确认平台仍 pending -> 撤销"。
func (a *auditorService) revokeForm(c context.Context, form model.AuditorForm) {
	status, err := a.PlatformStatus(c, form.Id)
	if err != nil {
		if errors.Is(err, ErrAuditorItemNotFound) {
			// 平台上已无该条目：无需撤销，按已完成处理，避免反复回查。
			a.l.Info("Expired auditor item already gone on platform",
				zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId))
			a.markRevoked(c, form)
			return
		}
		a.l.Error("Revoke expired auditor form: query platform status failed, will retry",
			zap.Error(err), zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId))
		return
	}
	switch status {
	case "pending":
		// 仍需撤销，继续往下走。
	case model.StancePass, model.StanceReject:
		// 平台已出结论：不撤销，标记完成即可。过期活动不再展示，本地结论没有意义。
		a.l.Info("Expired auditor item already concluded on platform, skip revoke",
			zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId), zap.String("status", status))
		a.markRevoked(c, form)
		return
	default:
		// 状态无法识别：不能确认是否该撤销，留待下一轮重试。
		a.l.Warn("Revoke expired auditor form: unknown platform status, skip",
			zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId), zap.String("status", status))
		return
	}

	if err := a.DeleteItem(c, form.Id); err != nil {
		// 平台以 HTTP 200 返回时，SDK 仍可能因 data 形状不符合预期（如 null）判定为
		// "响应数据格式错误"，而删除其实已经生效。回查一次确认：
		//   - 条目已不存在：删除已生效；
		//   - 平台已出结论：条目不再需要撤销。
		// 两种都标记完成，否则会每小时重复删除失败、重复告警且永不收敛。
		// 只有状态无法确认（网络错误、非法响应）才留待下一轮重试。
		status, statErr := a.PlatformStatus(c, form.Id)
		switch {
		case errors.Is(statErr, ErrAuditorItemNotFound):
			a.l.Info("Revoked expired auditor form (item gone after delete response)",
				zap.Error(err), zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId))
			a.markRevoked(c, form)
			return
		case statErr == nil && (status == model.StancePass || status == model.StanceReject):
			a.l.Info("Revoked expired auditor form (platform concluded after delete attempt)",
				zap.Error(err), zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId), zap.String("status", status))
			a.markRevoked(c, form)
			return
		case statErr == nil:
			a.l.Warn("Auditor item still on platform after delete attempt",
				zap.Error(err), zap.Int64("formId", form.Id), zap.String("status", status))
		}
		a.l.Error("Revoke expired auditor form failed, will retry",
			zap.Error(err), zap.Int64("formId", form.Id), zap.Int64("activityId", form.ActivityId))
		return
	}
	a.markRevoked(c, form)
}
