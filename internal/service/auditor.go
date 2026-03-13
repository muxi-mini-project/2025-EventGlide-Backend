package service

import (
	"github.com/gin-gonic/gin"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/client"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/dto"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"strings"
	"time"
)

var _ dao.AuditorRepository = (*dao.AuditorRepo)(nil)

type AuditorService interface {
	UploadForm(c *gin.Context, aw *req.AuditWrapper, FormId uint) error
	CreateAuditorForm(c *gin.Context, ActId, FormUrl, Sub string) (*model.AuditorForm, error)
}

type auditorService struct {
	ApiKey      string
	HookUrl     string
	MuxiCli     *client.Client
	AuditorRepo dao.AuditorRepository

	l *zap.Logger
}

func NewAuditorService(repo dao.AuditorRepository, l *zap.Logger) AuditorService {
	muxiCli, err := client.NewClient(client.Config{
		ApiKey: viper.GetString("auditor.apiKey"),
		Region: viper.GetString("auditor.region"),
	})
	if err != nil {
		l.Fatal("Failed to create Muxi Auditor client", zap.Error(err))
		panic(err)
	}

	c := &auditorService{
		ApiKey:      viper.GetString("auditor.apiKey"),
		HookUrl:     viper.GetString("auditor.hookUrl"),
		MuxiCli:     muxiCli,
		AuditorRepo: repo,
		l:           l.Named("auditor/service"),
	}
	return c
}

func (a *auditorService) UploadForm(c *gin.Context, aw *req.AuditWrapper, id uint) error {
	uploadReq := a.toUploadReq(aw, id)
	_, err := a.MuxiCli.UploadItem(c, &uploadReq)
	if err != nil {
		a.l.Error("Upload to auditor failed", zap.Error(err))
		return err
	}
	return nil
}

func (a *auditorService) CreateAuditorForm(c *gin.Context, ActId, FormUrl string, sub string) (*model.AuditorForm, error) {
	return a.AuditorRepo.Insert(c, ActId, FormUrl, sub)
}

func (a *auditorService) toUploadReq(aw *req.AuditWrapper, id uint) request.UploadReq {
	now := time.Now().Unix()
	res := request.UploadReq{
		HookUrl:    &a.HookUrl,
		Id:         &id,
		Tags:       &[]string{"校灵通"},
		PublicTime: &now,
	}

	if aw.Subject == SubjectActivity {
		author := extractAuthors(aw.CactReq.LabelForm.Signer)
		res.Author = &author
		*res.Tags = append(*res.Tags, aw.CactReq.LabelForm.Type, "活动")

		ctt := dto.NewContents(
			dto.WithTopicText(aw.CactReq.Title, aw.CactReq.Introduce),
			dto.WithTopicPictures(aw.CactReq.ShowImg),
		)
		res.Content = ctt

		if tools.IfRegisterMapper(aw.CactReq.LabelForm.IfRegister) {
			*res.Tags = append(*res.Tags, "含报名表需要审核")
			res.Content.Topic.Pictures = append(res.Content.Topic.Pictures, aw.CactReq.LabelForm.ActiveForm)
		}

	} else if aw.Subject == SubjectPost {
		res.Author = &aw.StudentId
		*res.Tags = append(*res.Tags, "帖子")

		ctt := dto.NewContents(
			dto.WithTopicText(aw.CpostReq.Title, aw.CpostReq.Introduce),
			dto.WithTopicPictures(aw.CpostReq.ShowImg),
		)
		res.Content = ctt
	}

	return res
}

func extractAuthors(signers []struct {
	StudentID string `json:"studentId" validate:"len=10"`
	Name      string `json:"name"`
}) string {
	builder := strings.Builder{}
	for _, s := range signers {
		builder.WriteString(s.Name + "-")
	}
	return builder.String()
}
