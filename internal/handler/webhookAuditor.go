package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/response"
	sdk "github.com/muxi-Infra/auditor-Backend/sdk/v2/server/gin"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

type CallbackAuditorHandler struct {
	svc service.CallbackAuditorService
	l   *zap.Logger
}

func NewCallbackAuditorHandler(e *gin.Engine, svc service.CallbackAuditorService, cfg *config.Conf, l *logger.LoggerSet) *CallbackAuditorHandler {
	c := &CallbackAuditorHandler{
		svc: svc,
		l:   l.Auditor.Named("handler"),
	}
	s := sdk.NewGinRegistrar(&e.RouterGroup)
	chain := sdk.NewChain()
	s.WebHook(cfg.Auditor.WebHookPath, chain, c.CallbackAuditor)

	return c
}

func (w *CallbackAuditorHandler) CallbackAuditor(c *gin.Context, req *request.HookPayload) (response.Resp, error) {
	if err := w.svc.UpdateStatus(c, int64(req.Data.Id), req.Data.Status); err != nil {
		w.l.Error("Failed to update auditor status", zap.Int64("id", int64(req.Data.Id)), zap.String("status", req.Data.Status), zap.Error(err))
		// 无效状态属永久性错误返回 400，其余（如 DB 故障）返回 500，供平台按其重试策略处理。
		// 返回 nil：SDK 包装层在 err != nil 时会再写一次响应体，形成非法 JSON；此处已直接写状态码。
		if errors.Is(err, errs.ErrAuditorStatusInvalid) {
			c.AbortWithStatus(http.StatusBadRequest)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return response.Resp{}, nil
	}

	w.l.Info("Auditor status updated successfully", zap.Int64("id", int64(req.Data.Id)), zap.String("status", req.Data.Status))
	return response.Resp{
		Code: 200,
		Msg:  "Success",
	}, nil
}
