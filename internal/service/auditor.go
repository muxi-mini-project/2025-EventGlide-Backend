package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdkerrorx "github.com/muxi-Infra/auditor-Backend/sdk/v2/api/errorx"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/client"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ dao.AuditorRepository = (*dao.AuditorRepo)(nil)

// ErrFormAlreadyPushed 表示该表单已成功送审，后台轮询应跳过，避免重复送审。
var ErrFormAlreadyPushed = errors.New("auditor form already pushed")

type AuditorService interface {
	UploadForm(c context.Context, aw *req.AuditWrapper, FormId int64) error
	CreateAuditorForm(c context.Context, ActId int64, FormUrl string, Sub string) (*model.AuditorForm, error)
	GetOrCreatePendingForm(c context.Context, ActId int64, FormUrl string, Sub string) (*model.AuditorForm, error)
	MarkPushed(c context.Context, FormId int64, pushedAt time.Time) error
}

type auditorService struct {
	ApiKey      string
	HookUrl     string
	MuxiCli     *client.Client
	AuditorRepo dao.AuditorRepository

	l *zap.Logger
}

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
		a.l.Error("Upload to auditor failed", zap.Error(err))
		return errs.ErrUploadFormFailed.Wrap(err)
	}
	if resp.Basic.Code != sdkerrorx.SuccessCode {
		err := fmt.Errorf("auditor rejected upload: code=%d msg=%s", resp.Basic.Code, resp.Basic.Msg)
		a.l.Error("Auditor upload not accepted", zap.Int("code", resp.Basic.Code), zap.String("msg", resp.Basic.Msg), zap.Int64("formId", id))
		return errs.ErrUploadFormFailed.Wrap(err)
	}
	return nil
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
