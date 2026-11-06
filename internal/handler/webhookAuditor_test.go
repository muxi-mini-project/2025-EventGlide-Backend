package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
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

func callCallback(t *testing.T, h *CallbackAuditorHandler, apiKeyHeader *string) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhook", nil)
	if apiKeyHeader != nil {
		c.Request.Header.Set("api_key", *apiKeyHeader)
	}

	_, err := h.CallbackAuditor(c, &request.HookPayload{
		Data: request.WebHookData{Id: 42, Status: "通过"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return c
}

func strPtr(s string) *string { return &s }

// TestCallbackAuditorAcceptsMatchingApiKey 正确的 api_key 应放行并调用 service。
func TestCallbackAuditorAcceptsMatchingApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	h := newCallbackAuditorHandlerForTest("secret-key", svc)

	c := callCallback(t, h, strPtr("secret-key"))

	if c.Writer.Status() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", c.Writer.Status())
	}
	if svc.calls != 1 {
		t.Fatalf("expected service called once, got %d", svc.calls)
	}
}

// TestCallbackAuditorRejectsWrongApiKey 错误的 api_key 应返回 401，且不得调用 service。
func TestCallbackAuditorRejectsWrongApiKey(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	h := newCallbackAuditorHandlerForTest("secret-key", svc)

	c := callCallback(t, h, strPtr("wrong-key"))

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

	c := callCallback(t, h, nil)

	if c.Writer.Status() != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", c.Writer.Status())
	}
	if svc.calls != 0 {
		t.Fatalf("service must not be called on missing key, got %d", svc.calls)
	}
}

// TestCallbackAuditorSkipsCheckWhenKeyUnset apiKey 未配置时跳过校验（防御分支），照常处理回调。
func TestCallbackAuditorSkipsCheckWhenKeyUnset(t *testing.T) {
	svc := &fakeCallbackAuditorSvc{}
	h := newCallbackAuditorHandlerForTest("", svc)

	c := callCallback(t, h, nil)

	if c.Writer.Status() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", c.Writer.Status())
	}
	if svc.calls != 1 {
		t.Fatalf("expected service called once, got %d", svc.calls)
	}
}
