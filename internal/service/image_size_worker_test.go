package service

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
)

// fakeImageSource 内存图片源，记录 worker 的查询与回写，便于断言。
type fakeImageSource struct {
	mu      sync.Mutex
	imgs    []model.Image
	updates map[int64][2]int
	copied  bool
}

func (f *fakeImageSource) FindZeroSizeImages(context.Context, int) ([]model.Image, error) {
	return f.imgs, nil
}

func (f *fakeImageSource) UpdateImageSize(_ context.Context, id int64, w, h int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updates == nil {
		f.updates = map[int64][2]int{}
	}
	f.updates[id] = [2]int{w, h}
	return nil
}

func (f *fakeImageSource) CopySizeFromDuplicates(context.Context) error {
	f.copied = true
	return nil
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// testResolver 用测试图床域名构造 resolver（白名单即 httptest 地址）。
func testResolver(srv *httptest.Server) *ImageSizeResolver {
	return &ImageSizeResolver{
		client:      &http.Client{Timeout: imageSizeFetchTimeout},
		allowedHost: hostFromURL(srv.URL),
	}
}

// newImgServer 按路径返回不同尺寸的 PNG，hits 记录请求次数（含未注册路径的 404）。
func newImgServer(t *testing.T, paths map[string][]byte, hits *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, data := range paths {
		data := data
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(data)
		})
	}
	// 计数放外层中间件：mux 对未注册路径直接回 404 不经过 handler，放 handler 里会漏计。
	counting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(counting)
	t.Cleanup(srv.Close)
	return srv
}

// TestImageSizeWorkerProcessBatchFillsSizes 正常路径：拉取原图解析宽高并按行回写，且先复制重复行。
func TestImageSizeWorkerProcessBatchFillsSizes(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{
		"/a.png": pngBytes(t, 32, 16),
		"/b.png": pngBytes(t, 8, 4),
	}, &hits)

	resolver := testResolver(srv)
	src := &fakeImageSource{imgs: []model.Image{
		{Id: 1, Url: srv.URL + "/a.png"},
		{Id: 2, Url: srv.URL + "/b.png"},
	}}
	w := &ImageSizeWorker{source: src, resolver: resolver, l: zap.NewNop()}

	w.processBatch()

	if !src.copied {
		t.Error("CopySizeFromDuplicates 未被调用，重复 URL 不会被免费回填")
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [2]int{32, 16} {
		t.Errorf("image 1 回写 = %v, want {32 16}", got)
	}
	if got := src.updates[2]; got != [2]int{8, 4} {
		t.Errorf("image 2 回写 = %v, want {8 4}", got)
	}
}

// TestImageSizeWorkerProcessBatchMarksFailures 404 与非白名单主机都标记 -1，
// 且非白名单 URL 不会发起请求（防 SSRF）。
func TestImageSizeWorkerProcessBatchMarksFailures(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{"/a.png": pngBytes(t, 32, 16)}, &hits)

	resolver := testResolver(srv)
	src := &fakeImageSource{imgs: []model.Image{
		{Id: 1, Url: srv.URL + "/missing.png"},       // 404
		{Id: 2, Url: "http://169.254.169.254/x.png"}, // 非白名单主机，不得请求
	}}
	w := &ImageSizeWorker{source: src, resolver: resolver, l: zap.NewNop()}

	w.processBatch()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("图床服务被请求 %d 次, want 1（非白名单 URL 不得发起请求）", got)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [2]int{-1, -1} {
		t.Errorf("404 行回写 = %v, want {-1 -1}", got)
	}
	if got := src.updates[2]; got != [2]int{-1, -1} {
		t.Errorf("非白名单行回写 = %v, want {-1 -1}", got)
	}
}

// TestImageSizeResolverResolveMissing 创建路径同步兜底：
// 已有宽高的行不重复拉取，0 宽高行原地填充，失败行保持 0。
func TestImageSizeResolverResolveMissing(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{"/a.png": pngBytes(t, 32, 16)}, &hits)

	resolver := testResolver(srv)
	imgs := []model.Image{
		{Url: srv.URL + "/a.png"},
		{Url: srv.URL + "/missing.png"},
		{Url: srv.URL + "/pre.png", Width: 100, Height: 50},
	}

	resolver.ResolveMissing(context.Background(), imgs)

	if got := [2]int{imgs[0].Width, imgs[0].Height}; got != [2]int{32, 16} {
		t.Errorf("第一张宽高 = %v, want {32 16}", got)
	}
	if got := [2]int{imgs[1].Width, imgs[1].Height}; got != [2]int{0, 0} {
		t.Errorf("失败行宽高 = %v, want 保持 {0 0} 交 worker 重试", got)
	}
	if got := [2]int{imgs[2].Width, imgs[2].Height}; got != [2]int{100, 50} {
		t.Errorf("前端已传宽高的行被覆盖 = %v, want {100 50}", got)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("图床服务被请求 %d 次, want 2（已有宽高的行不得重复拉取）", got)
	}
}
