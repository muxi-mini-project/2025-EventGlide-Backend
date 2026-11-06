package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestHealthz 容器 HEALTHCHECK 依赖这个端点，必须稳定返回 200 且不依赖 DB/Redis。
func TestHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &Handler{}
	h.healthz(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Fatalf("body = %q, want contains ok", w.Body.String())
	}
}
