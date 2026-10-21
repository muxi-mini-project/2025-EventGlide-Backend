package ginx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/pkg/errorx"
	"github.com/raiki02/EG/pkg/utils"
)

type testCreateReq struct {
	Title string `json:"title" validate:"required"`
}

type testSnowflakeReq struct {
	ID utils.SnowflakeID `json:"id" validate:"required"`
}

type testQueryReq struct {
	Page int `json:"page" form:"page"`
}

func serve(t *testing.T, method, route, target string, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := gin.New()
	r.Handle(method, route, handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) resp.Resp {
	t.Helper()

	var got resp.Resp
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body %q: %v", w.Body.String(), err)
	}
	return got
}

func TestWrapRequestReturnsParamInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	handler := WrapRequest(func(c *gin.Context, req testCreateReq) (resp.Resp, error) {
		return ReturnSuccess(nil)
	})

	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{name: "missing field", body: `{}`, wantMsg: "参数错误: title 不能为空"},
		{name: "wrong type", body: `{"title":123}`, wantMsg: "参数错误: title 类型错误"},
		{name: "broken json", body: `not-json`, wantMsg: "参数错误: 请求体不是合法的 JSON"},
		{name: "truncated json", body: `{"title":`, wantMsg: "参数错误: 请求体不是合法的 JSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(t, http.MethodPost, "/x", "/x", handler, tt.body)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			got := decodeResp(t, w)
			if got.Code != errs.ParamInvalid {
				t.Fatalf("code = %d, want %d", got.Code, errs.ParamInvalid)
			}
			if got.Msg != tt.wantMsg {
				t.Fatalf("msg = %q, want %q", got.Msg, tt.wantMsg)
			}
		})
	}
}

func TestWrapRequestReturnsParamInvalidForURIParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	handler := WrapRequest(func(c *gin.Context, req req.FindActByIdReq) (resp.Resp, error) {
		return ReturnSuccess(nil)
	})

	w := serve(t, http.MethodGet, "/x/:id", "/x/abc", handler, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	got := decodeResp(t, w)
	if got.Code != errs.ParamInvalid || got.Msg != "参数错误: 路径参数格式错误" {
		t.Fatalf("resp = %+v, want uri param error", got)
	}
}

func TestWrapRequestBindsQueryParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	pageHandler := WrapRequest(func(c *gin.Context, req testQueryReq) (resp.Resp, error) {
		return ReturnSuccess(req.Page)
	})

	// JSON 往返后数字是 float64
	pages := []struct {
		name  string
		query string
		want  float64
	}{
		{name: "valid value", query: "?page=2", want: 2},
		{name: "empty value", query: "?page=", want: 0},
		{name: "repeated value", query: "?page=1&page=2", want: 1},
	}
	for _, tt := range pages {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(t, http.MethodGet, "/x", "/x"+tt.query, pageHandler, "")
			got := decodeResp(t, w)
			if w.Code != http.StatusOK || got.Code != http.StatusOK || got.Data != tt.want {
				t.Fatalf("resp = %+v, want page %v", got, tt.want)
			}
		})
	}

	assertQueryParamError := func(t *testing.T, w *httptest.ResponseRecorder) {
		t.Helper()

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
		got := decodeResp(t, w)
		if got.Code != errs.ParamInvalid || got.Msg != "参数错误: 查询参数格式错误" {
			t.Fatalf("resp = %+v, want query param error", got)
		}
	}

	t.Run("invalid value", func(t *testing.T) {
		assertQueryParamError(t, serve(t, http.MethodGet, "/x", "/x?page=abc", pageHandler, ""))
	})
}

func TestWrapRequestBindsURIParamOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	handler := WrapRequest(func(c *gin.Context, req_ req.FindActByIdReq) (resp.Resp, error) {
		return ReturnSuccess(req_.Id)
	})

	targets := []string{
		"/x/123",
		"/x/123?id=456",
		"/x/123?id=",
		"/x/123?id=abc",
		"/x/123?ID=abc",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			w := serve(t, http.MethodGet, "/x/:id", target, handler, "")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
			}
			got := decodeResp(t, w)
			if got.Code != http.StatusOK || got.Data != "123" {
				t.Fatalf("resp = %+v, want path id 123", got)
			}
		})
	}
}

func TestWrapRequestReturnsParamInvalidForCustomUnmarshal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	handler := WrapRequest(func(c *gin.Context, req testSnowflakeReq) (resp.Resp, error) {
		return ReturnSuccess(nil)
	})

	w := serve(t, http.MethodPost, "/x", "/x", handler, `{"id":123}`)
	got := decodeResp(t, w)
	if got.Code != errs.ParamInvalid || got.Msg != "参数错误: 请求体格式错误" {
		t.Fatalf("resp = %+v, want body param error", got)
	}
}

func TestWrapRequestKeepsBusinessError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	handler := WrapRequest(func(c *gin.Context, req testCreateReq) (resp.Resp, error) {
		return resp.Resp{}, errorx.New(errs.ActivityNotFound, "活动不存在")
	})

	w := serve(t, http.MethodPost, "/x", "/x", handler, `{"title":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	got := decodeResp(t, w)
	if got.Code != errs.ActivityNotFound || got.Msg != "活动不存在" {
		t.Fatalf("resp = %+v, want business error", got)
	}
}

func TestWrapRequestKeepsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitValidation()

	tests := []struct {
		name string
		err  error
	}{
		{name: "plain error", err: errors.New("db down")},
		{name: "io eof from service", err: io.EOF},
		{name: "json syntax error from service", err: &json.SyntaxError{Offset: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := WrapRequest(func(c *gin.Context, req testCreateReq) (resp.Resp, error) {
				return resp.Resp{}, tt.err
			})

			w := serve(t, http.MethodPost, "/x", "/x", handler, `{"title":"x"}`)
			got := decodeResp(t, w)
			if got.Code != errs.InternalError || got.Msg != "服务器错误" {
				t.Fatalf("resp = %+v, want internal error", got)
			}
		})
	}
}
