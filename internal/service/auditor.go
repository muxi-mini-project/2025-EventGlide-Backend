package service

import (
	"context"

	"github.com/muxi-Infra/auditor-Backend/sdk/v2/client"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/ioc"
	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
)

var _ dao.AuditorRepository = (*dao.AuditorRepo)(nil)

type AuditorService interface {
	UploadForm(c context.Context, aw *req.AuditWrapper, FormId uint) error
	CreateAuditorForm(c context.Context, ActId, FormUrl, Sub string) (*model.AuditorForm, error)
}

type auditorService struct {
	ApiKey      string
	HookUrl     string
	MuxiCli     *client.Client
	AuditorRepo dao.AuditorRepository

	l *zap.Logger
}

func NewAuditorService(repo dao.AuditorRepository, ls *ioc.LoggerSet, cfg *config.Conf) AuditorService {
	muxiCli, err := client.NewClient(client.Config{
		ApiKey: cfg.Auditor.ApiKey,
		Region: cfg.Auditor.Region,
	})
	if err != nil {
		ls.Auditor.Fatal("Failed to create Muxi Auditor client", zap.Error(err))
		panic(err)
	}

	c := &auditorService{
		ApiKey:      cfg.Auditor.ApiKey,
		HookUrl:     cfg.Auditor.HookURL,
		MuxiCli:     muxiCli,
		AuditorRepo: repo,
		l:           ls.Auditor.Named("service"),
	}
	return c
}

func (a *auditorService) UploadForm(c context.Context, aw *req.AuditWrapper, id uint) error {
	uploadReq := converter.AuditorUploadReqFromWrapper(aw, id, a.HookUrl)
	_, err := a.MuxiCli.UploadItem(c, &uploadReq)
	if err != nil {
		a.l.Error("Upload to auditor failed", zap.Error(err))
		return err
	}
	return nil
}

func (a *auditorService) CreateAuditorForm(c context.Context, ActId, FormUrl string, sub string) (*model.AuditorForm, error) {
	return a.AuditorRepo.Insert(c, ActId, FormUrl, sub)
}
