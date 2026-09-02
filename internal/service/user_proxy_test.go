package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/raiki02/EG/internal/errs"
)

func TestCCNUTransportProxiesLoginHosts(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}

	transport := newCCNUTransport(proxyURL)
	tests := []struct {
		name      string
		targetURL string
		wantProxy bool
	}{
		{name: "account service uses proxy", targetURL: "https://account.ccnu.edu.cn/cas/login", wantProxy: true},
		{name: "postgraduate service uses proxy", targetURL: "https://grd.ccnu.edu.cn/yjsxt/", wantProxy: true},
		{name: "academic service connects directly", targetURL: "https://bkzhjw.ccnu.edu.cn/jsxsd/", wantProxy: false},
		{name: "unrelated host connects directly", targetURL: "https://example.com/", wantProxy: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tt.targetURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := transport.Proxy(req)
			if err != nil {
				t.Fatal(err)
			}
			if (got != nil) != tt.wantProxy {
				t.Fatalf("proxy = %v, wantProxy = %v", got, tt.wantProxy)
			}
		})
	}
}

func TestShouldRetryLoginDirect(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "proxy provider error", err: &loginProxyError{err: errors.New("unavailable")}, want: true},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "network error", err: errs.ErrNetworkError, want: true},
		{name: "invalid login page", err: errs.ErrLoginInfoInvalid, want: true},
		{name: "bad credentials", err: errs.ErrLoginFailed, want: false},
		{name: "unrelated error", err: errors.New("boom"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryLoginDirect(tt.err); got != tt.want {
				t.Fatalf("shouldRetryLoginDirect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShenlongProxyProviderInvalidate(t *testing.T) {
	p := &shenlongProxyProvider{
		proxyAddr: "http://127.0.0.1:18080",
		expiresAt: time.Now().Add(time.Minute),
	}

	p.invalidate()

	if p.proxyAddr != "" {
		t.Fatalf("proxyAddr = %q, want empty", p.proxyAddr)
	}
	if !p.expiresAt.IsZero() {
		t.Fatalf("expiresAt = %v, want zero", p.expiresAt)
	}
}
