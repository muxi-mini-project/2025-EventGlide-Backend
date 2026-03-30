package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/response"
	sdk "github.com/muxi-Infra/auditor-Backend/sdk/v2/server/gin"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/service"
	"go.uber.org/zap"
)

type CallbackAuditorController struct {
	svc service.CallbackAuditorService
	l   *zap.Logger
}

func NewCallbackAuditorController(e *gin.Engine, svc service.CallbackAuditorService, cfg *config.Conf) *CallbackAuditorController {
	c := &CallbackAuditorController{
		svc: svc,
		l:   zap.L().Named("callbackAuditor/controller"),
	}
	s := sdk.NewGinRegistrar(&e.RouterGroup)
	chain := sdk.NewChain()
	s.WebHook(cfg.Auditor.WebHookPath, chain, c.CallbackAuditor)

	return c
}

func (w *CallbackAuditorController) CallbackAuditor(c *gin.Context, req *request.HookPayload) (response.Resp, error) {

	if err := w.svc.UpdateStatus(c, req.Data.Id, req.Data.Status); err != nil {
		w.l.Error("Failed to update auditor status", zap.Uint("id", req.Data.Id), zap.String("status", req.Data.Status), zap.Error(err))
	} else {
		w.l.Info("Auditor status updated successfully", zap.Uint("id", req.Data.Id), zap.String("status", req.Data.Status))
	}
	return response.Resp{
		Code: 200,
		Msg:  "Success",
	}, nil
}
