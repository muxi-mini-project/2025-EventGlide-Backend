package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/response"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

type fakeCallbackAuditorSvc struct {
	calls int
}

func (f *fakeCallbackAuditorSvc) UpdateStatus(context.Context, int64, string, string) error {
	f.calls++
	return nil
}

func newCallbackAuditorHandlerForTest(apiKey string, svc *fakeCallbackAuditorSvc) *CallbackAuditorHandler {
	return &CallbackAuditorHandler{
		svc:    svc,
		apiKey: apiKey,
		l:      zap.NewNop(),
	}
}

func callCallback(t *testing.T, h *CallbackAuditorHandler, apiKeyHeader *string) (*gin.Context, response.Resp) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook", nil)
	if apiKeyHeader != nil {
		c.Request.Header.Set("api_key", *apiKeyHeader)
	}

	resp, err := h.CallbackAuditor(c, &request.HookPayload{
		Data: request.WebHookData{Id: 42, Status: "通过"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return c, resp
}

// assertAccepted 断言放行时返回的响应体，避免只依赖 gin 默认 200（未写响应时也是 200）。
func assertAccepted(t *testing.T, resp response.Resp) {
	t.Helper()
	if resp.Code != 200 || resp.Msg != "Success" {
		t.Fatalf("expected accepted resp {200 Success}, got %+v", resp)
	}
}

func strPtr(s string) *string { return &s }

// TestCallbackAuditorAcceptsMatchingApiKey 正确的 api_key 应放行并调用 service。
func TestCallbackAuditorAcceptsMatchingApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	h := newCallbackAuditorHandlerForTest("secret-key", svc)

	_, resp := callCallback(t, h, strPtr("secret-key"))

	assertAccepted(t, resp)
	if svc.calls != 1 {
		t.Fatalf("expected service called once, got %d", svc.calls)
	}
}

// TestCallbackAuditorRejectsWrongApiKey 错误的 api_key 应返回 401，且不得调用 service。
func TestCallbackAuditorRejectsWrongApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	h := newCallbackAuditorHandlerForTest("secret-key", svc)

	c, _ := callCallback(t, h, strPtr("wrong-key"))

	if c.Writer.Status() != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", c.Writer.Status())
	}
	if svc.calls != 0 {
		t.Fatalf("service must not be called on auth failure, got %d", svc.calls)
	}
}

// TestCallbackAuditorRejectsMissingApiKey 缺失 api_key 头应返回 401，且不得调用 service。
func TestCallbackAuditorRejectsMissingApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	h := newCallbackAuditorHandlerForTest("secret-key", svc)

	c, _ := callCallback(t, h, nil)

	if c.Writer.Status() != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", c.Writer.Status())
	}
	if svc.calls != 0 {
		t.Fatalf("service must not be called on missing key, got %d", svc.calls)
	}
}

// TestCallbackAuditorRouteRejectsWhenKeyUnset 走真实 SDK 路由：apiKey 未配置时 fail-closed，
// 返回 500 且不得调用 service。
func TestCallbackAuditorRouteRejectsWhenKeyUnset(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	engine := newCallbackAuditorRouteEngine(t, "", svc)

	w := postCallbackRoute(t, engine, nil)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.calls != 0 {
		t.Fatalf("service must not be called when key is unset, got %d", svc.calls)
	}
}

// newCallbackAuditorRouteEngine 通过 NewCallbackAuditorHandler 注册 SDK 路由，
// 覆盖从真实 HTTP 请求到 handler 的完整链路（含 SDK 的响应包装）。
func newCallbackAuditorRouteEngine(t *testing.T, apiKey string, svc *fakeCallbackAuditorSvc) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	NewCallbackAuditorHandler(engine, svc, &config.Conf{Auditor: config.AuditorConf{
		ApiKey:      apiKey,
		WebHookPath: "/hook",
	}}, logger.NewLoggerSet())
	return engine
}

// postCallbackRoute 向注册好的路由发一次真实回调请求。
func postCallbackRoute(t *testing.T, engine *gin.Engine, apiKeyHeader *string) *httptest.ResponseRecorder {
	t.Helper()

	body := strings.NewReader(`{"event":"audit","try":1,"data":{"id":42,"status":"通过","msg":""}}`)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", body)
	req.Header.Set("Content-Type", "application/json")
	if apiKeyHeader != nil {
		req.Header.Set("api_key", *apiKeyHeader)
	}

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// TestCallbackAuditorRouteAcceptsMatchingApiKey 走真实 SDK 路由：正确 api_key 应 200 并调用 service 一次。
func TestCallbackAuditorRouteAcceptsMatchingApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	engine := newCallbackAuditorRouteEngine(t, "secret-key", svc)

	w := postCallbackRoute(t, engine, strPtr("secret-key"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.calls != 1 {
		t.Fatalf("expected service called once, got %d", svc.calls)
	}
}

// TestCallbackAuditorRouteRejectsWrongApiKey 走真实 SDK 路由：错误 api_key 应 401 且不调用 service。
// 同时固定 SDK 包装层的现状：HTTP 状态是 401，但响应体仍是 {"code":200,"msg":"success",...}，
// 平台只能靠状态码判定失败。
func TestCallbackAuditorRouteRejectsWrongApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	engine := newCallbackAuditorRouteEngine(t, "secret-key", svc)

	w := postCallbackRoute(t, engine, strPtr("wrong-key"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.calls != 0 {
		t.Fatalf("service must not be called on auth failure, got %d", svc.calls)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":200`) || !strings.Contains(body, `"msg":"success"`) {
		t.Fatalf("expected SDK-wrapped success body with 401 status, got %s", body)
	}
}

// TestCallbackAuditorRouteRejectsMissingApiKey 走真实 SDK 路由：缺失 api_key 头应 401 且不调用 service。
func TestCallbackAuditorRouteRejectsMissingApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	engine := newCallbackAuditorRouteEngine(t, "secret-key", svc)

	w := postCallbackRoute(t, engine, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.calls != 0 {
		t.Fatalf("service must not be called on missing key, got %d", svc.calls)
	}
}
