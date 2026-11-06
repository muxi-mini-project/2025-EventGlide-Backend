package handler

import (
	"crypto/subtle"
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
	svc    service.CallbackAuditorService
	apiKey string
	l      *zap.Logger
}

func NewCallbackAuditorHandler(e *gin.Engine, svc service.CallbackAuditorService, cfg *config.Conf, l *logger.LoggerSet) *CallbackAuditorHandler {
	c := &CallbackAuditorHandler{
		svc:    svc,
		apiKey: cfg.Auditor.ApiKey,
		l:      l.Auditor.Named("handler"),
	}
	s := sdk.NewGinRegistrar(&e.RouterGroup)
	chain := sdk.NewChain()
	s.WebHook(cfg.Auditor.WebHookPath, chain, c.CallbackAuditor)

	return c
}

func (w *CallbackAuditorHandler) CallbackAuditor(c *gin.Context, req *request.HookPayload) (response.Resp, error) {
	if w.apiKey == "" {
		// fail-closed：未配置 key 时宁可让回调失败，也不在无鉴权的情况下接受。
		// 该分支正常启动不可达：NewAuditorService 构造时 client.NewClient 会因空 key
		// 报错并 Fatal+panic，这里仅作防御。
		w.l.Error("Auditor callback api_key not configured, reject callback")
		c.AbortWithStatus(http.StatusInternalServerError)
		return response.Resp{}, nil
	}

	got := c.GetHeader("api_key")
	// 平台回调以 api_key 头携带项目 API Key，必须校验，否则任何人都能伪造审核结论。
	if subtle.ConstantTimeCompare([]byte(got), []byte(w.apiKey)) != 1 {
		w.l.Warn("Auditor callback rejected: api_key mismatch")
		// 平台按 HTTP 状态判断失败并重试；响应体由 SDK 包装层统一写成 {"code":200,"msg":"success"}，
		// 这里无法覆盖，能保证的只有 401 状态码。
		c.AbortWithStatus(http.StatusUnauthorized)
		return response.Resp{}, nil
	}

	if err := w.svc.UpdateStatus(c, int64(req.Data.Id), req.Data.Status, req.Data.Msg); err != nil {
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

	w.l.Info("Auditor status updated successfully",
		zap.Int64("id", int64(req.Data.Id)),
		zap.String("status", req.Data.Status),
		zap.Int("reasonLen", len(req.Data.Msg)),
	)
	return response.Resp{
		Code: 200,
		Msg:  "Success",
	}, nil
}
