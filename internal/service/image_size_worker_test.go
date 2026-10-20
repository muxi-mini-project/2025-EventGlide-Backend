package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
)

// fakeImageSource 内存图片源，记录 worker 的查询与回写，便于断言。
type fakeImageSource struct {
	mu             sync.Mutex
	imgs           []model.Image
	updates        map[int64][2]int
	updateDeadline time.Time // 最近一次回写收到的 ctx deadline
	copied         bool
}

func (f *fakeImageSource) FindZeroSizeImages(context.Context, int) ([]model.Image, error) {
	return f.imgs, nil
}

func (f *fakeImageSource) UpdateImageSize(ctx context.Context, id int64, w, h int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updates == nil {
		f.updates = map[int64][2]int{}
	}
	f.updates[id] = [2]int{w, h}
	if d, ok := ctx.Deadline(); ok {
		f.updateDeadline = d
	}
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

// testResolver 用测试图床域名构造 resolver（白名单即 httptest 地址，同样拦截跨域重定向）。
func testResolver(srv *httptest.Server) *ImageSizeResolver {
	return &ImageSizeResolver{
		client:      newImageSizeClient(hostFromURL(srv.URL)),
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
	srv := newImgServer(t, map[string][]byte{
		"/a.png":       pngBytes(t, 32, 16),
		"/partial.png": pngBytes(t, 20, 10),
	}, &hits)

	resolver := testResolver(srv)
	imgs := []model.Image{
		{Url: srv.URL + "/a.png"},
		{Url: srv.URL + "/missing.png"},
		{Url: srv.URL + "/pre.png", Width: 100, Height: 50},
		{Url: srv.URL + "/partial.png", Width: 100, Height: 0},
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
	if got := [2]int{imgs[3].Width, imgs[3].Height}; got != [2]int{20, 10} {
		t.Errorf("部分宽高行 = %v, want 补全为 {20 10}", got)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("图床服务被请求 %d 次, want 3（完整宽高行不得重复拉取，部分宽高行需要拉取）", got)
	}
}

// TestImageSizeResolverBlocksCrossHostRedirect 白名单域 302 到其他域必须被拒绝且目标零请求（防 SSRF）。
func TestImageSizeResolverBlocksCrossHostRedirect(t *testing.T) {
	var hits int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(pngBytes(t, 8, 4))
	}))
	t.Cleanup(other.Close)

	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/b.png", http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)

	resolver := testResolver(redirecting)
	_, _, err := resolver.Fetch(context.Background(), redirecting.URL+"/a.png")
	if !errors.Is(err, errImagePermanent) {
		t.Errorf("跨域重定向 err = %v, want wrap errImagePermanent", err)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("非白名单重定向目标被请求 %d 次, want 0", got)
	}
}

// TestImageSizeWorkerProcessBatchKeepsTransientFailures 超时属临时失败：
// 不标 -1，保持 0 待下轮重试（避免图床抖动导致永久丢宽高）。
func TestImageSizeWorkerProcessBatchKeepsTransientFailures(t *testing.T) {
	old := imageSizeFetchTimeout
	imageSizeFetchTimeout = 100 * time.Millisecond
	t.Cleanup(func() { imageSizeFetchTimeout = old })

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond) // 远超 100ms 超时，触发超时错误
		_, _ = w.Write(pngBytes(t, 32, 16))
	}))
	t.Cleanup(slow.Close)

	resolver := testResolver(slow)
	src := &fakeImageSource{imgs: []model.Image{{Id: 1, Url: slow.URL + "/slow.png"}}}
	w := &ImageSizeWorker{source: src, resolver: resolver, l: zap.NewNop()}

	w.processBatch()

	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.updates) != 0 {
		t.Errorf("超时失败被回写 = %v, want 无回写（保持 0 待重试）", src.updates)
	}
}

// TestImageSizeWorkerWritesWithFreshCtx 回写必须使用独立于拉图 deadline 的 ctx：
// 慢图耗尽拉图预算后，复用拉图 ctx 回写会随 deadline 一起失败，行保持 0 每 tick 死链重试
func TestImageSizeWorkerWritesWithFreshCtx(t *testing.T) {
	oldFetch, oldWrite := imageSizeFetchTimeout, imageSizeDBWriteTimeout
	imageSizeFetchTimeout = 500 * time.Millisecond
	imageSizeDBWriteTimeout = 2 * time.Second
	t.Cleanup(func() { imageSizeFetchTimeout = oldFetch; imageSizeDBWriteTimeout = oldWrite })

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond) // 拉图消耗 fctx 大部分预算但能成功
		_, _ = w.Write(pngBytes(t, 32, 16))
	}))
	t.Cleanup(slow.Close)

	resolver := testResolver(slow)
	src := &fakeImageSource{imgs: []model.Image{{Id: 1, Url: slow.URL + "/slow.png"}}}
	w := &ImageSizeWorker{source: src, resolver: resolver, l: zap.NewNop()}

	w.processBatch()

	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [2]int{32, 16} {
		t.Fatalf("回写 = %v, want {32 16}", got)
	}
	if remain := time.Until(src.updateDeadline); remain < imageSizeDBWriteTimeout/2 {
		t.Errorf("回写 ctx 剩余 %v, 期望独立的 %v 预算；复用拉图 ctx 会导致回写随拉图超时一起失败", remain, imageSizeDBWriteTimeout)
	}
}
