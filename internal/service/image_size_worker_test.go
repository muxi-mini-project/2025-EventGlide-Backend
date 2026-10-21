package service

import (
	"bytes"
	"context"
	"encoding/base64"
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
	updates        map[int64][3]int // {width, height, size_status}
	updateDeadline time.Time        // 最近一次回写收到的 ctx deadline
	copied         bool
	autoDrain      bool // 回写后从待解析集移除该行（模拟 status 变更后的下轮查询）
	blockFind      bool // 阻塞 FindZeroSizeImages 直到 ctx 取消（模拟卡死的查询）
	finds          int  // FindZeroSizeImages 调用次数
}

func (f *fakeImageSource) FindZeroSizeImages(ctx context.Context, _ int) ([]model.Image, error) {
	if f.blockFind {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds++
	out := make([]model.Image, len(f.imgs))
	copy(out, f.imgs)
	return out, nil
}

func (f *fakeImageSource) UpdateImageSize(ctx context.Context, id int64, w, h, status int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updates == nil {
		f.updates = map[int64][3]int{}
	}
	f.updates[id] = [3]int{w, h, status}
	if d, ok := ctx.Deadline(); ok {
		f.updateDeadline = d
	}
	if f.autoDrain {
		for i, img := range f.imgs {
			if img.Id == id {
				f.imgs = append(f.imgs[:i], f.imgs[i+1:]...)
				break
			}
		}
	}
	return nil
}

func (f *fakeImageSource) CopySizeFromDuplicates(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = true
	return nil
}

// addImgs 新增待解析行（模拟创建路径落库后唤醒）。
func (f *fakeImageSource) addImgs(imgs ...model.Image) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.imgs = append(f.imgs, imgs...)
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func webpBytes(t *testing.T) []byte {
	t.Helper()
	d, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatalf("decode webp fixture: %v", err)
	}
	return d
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

	w.processBatch(context.Background())

	if !src.copied {
		t.Error("CopySizeFromDuplicates 未被调用，重复 URL 不会被免费回填")
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [3]int{32, 16, 1} {
		t.Errorf("image 1 回写 = %v, want {32 16 1}", got)
	}
	if got := src.updates[2]; got != [3]int{8, 4, 1} {
		t.Errorf("image 2 回写 = %v, want {8 4 1}", got)
	}
}

// TestImageSizeWorkerProcessBatchMarksFailures 404 与非白名单主机都置 failed（0/0 + status=2），
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

	w.processBatch(context.Background())

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("图床服务被请求 %d 次, want 1（非白名单 URL 不得发起请求）", got)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [3]int{0, 0, 2} {
		t.Errorf("404 行回写 = %v, want {0 0 2}", got)
	}
	if got := src.updates[2]; got != [3]int{0, 0, 2} {
		t.Errorf("非白名单行回写 = %v, want {0 0 2}", got)
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
// 不置 failed，保持 pending 待下轮重试（避免图床抖动导致永久丢宽高）。
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

	w.processBatch(context.Background())

	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.updates) != 0 {
		t.Errorf("超时失败被回写 = %v, want 无回写（保持 0 待重试）", src.updates)
	}
}

// TestImageSizeWorkerWritesWithFreshCtx 回写必须使用独立于拉图 deadline 的 ctx：
// 慢图耗尽拉图预算后，复用拉图 ctx 回写会随 deadline 一起失败，行保持 pending 每 tick 死链重试
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

	w.processBatch(context.Background())

	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [3]int{32, 16, 1} {
		t.Fatalf("回写 = %v, want {32 16 1}", got)
	}
	if remain := time.Until(src.updateDeadline); remain < imageSizeDBWriteTimeout/2 {
		t.Errorf("回写 ctx 剩余 %v, 期望独立的 %v 预算；复用拉图 ctx 会导致回写随拉图超时一起失败", remain, imageSizeDBWriteTimeout)
	}
}

func TestImageSizeResolverFetchesWebp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webpBytes(t))
	}))
	t.Cleanup(srv.Close)

	resolver := testResolver(srv)
	width, height, err := resolver.Fetch(context.Background(), srv.URL+"/a.webp")
	if err != nil {
		t.Fatalf("webp 解析失败 = %v, want 成功", err)
	}
	if width != 1 || height != 1 {
		t.Errorf("webp 宽高 = {%d %d}, want {1 1}", width, height)
	}
}

// waitFor 轮询等待条件满足，超时失败。worker 生命周期是异步状态迁移，需要轮询断言。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waitFor: 条件在 %v 内未满足", timeout)
}

// newLifecycleWorker 构造按需启停的 worker，tick 间隔缩短到 20ms 便于测试。
func newLifecycleWorker(t *testing.T, src *fakeImageSource, resolver *ImageSizeResolver) *ImageSizeWorker {
	t.Helper()
	old := imageSizeTickInterval
	imageSizeTickInterval = 20 * time.Millisecond
	t.Cleanup(func() { imageSizeTickInterval = old })
	return &ImageSizeWorker{source: src, resolver: resolver, l: zap.NewNop()}
}

// TestImageSizeWorkerDrainsAndCloses 核心生命周期：启动解析全部存量 pending，
// 回写数据库后待解析清空，worker 自动关闭。
func TestImageSizeWorkerDrainsAndCloses(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{
		"/a.png": pngBytes(t, 32, 16),
		"/b.png": pngBytes(t, 8, 4),
	}, &hits)

	resolver := testResolver(srv)
	src := &fakeImageSource{autoDrain: true, imgs: []model.Image{
		{Id: 1, Url: srv.URL + "/a.png"},
		{Id: 2, Url: srv.URL + "/b.png"},
	}}
	w := newLifecycleWorker(t, src, resolver)

	w.Start()

	waitFor(t, 2*time.Second, func() bool {
		src.mu.Lock()
		defer src.mu.Unlock()
		return src.updates[1] == [3]int{32, 16, 1} && src.updates[2] == [3]int{8, 4, 1}
	})
	// 解析完毕回写数据库后，worker 自动关闭。
	waitFor(t, 2*time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.running
	})
}

// TestImageSizeWorkerClosesImmediatelyWhenNoPending 无待解析数据（含无存量）不启动：
// 首轮即查，0 行立即关闭，不拉图不空转。
func TestImageSizeWorkerClosesImmediatelyWhenNoPending(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{"/a.png": pngBytes(t, 32, 16)}, &hits)

	resolver := testResolver(srv)
	src := &fakeImageSource{} // 无任何待解析行（含无存量）
	w := newLifecycleWorker(t, src, resolver)

	w.Start()

	waitFor(t, 2*time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.running
	})
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("无待解析数据却拉图 %d 次, want 0", got)
	}
	src.mu.Lock()
	finds := src.finds
	src.mu.Unlock()
	if finds != 1 {
		t.Errorf("FindZeroSizeImages 调用 %d 次, want 1（首轮即查即关，不空转）", finds)
	}
}

// TestImageSizeWorkerNotifyWakesClosedWorker 关闭后新落库 pending 行：Notify 重新拉起，
// 解析回写后再次自动关闭。
func TestImageSizeWorkerNotifyWakesClosedWorker(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{"/a.png": pngBytes(t, 32, 16)}, &hits)

	resolver := testResolver(srv)
	src := &fakeImageSource{autoDrain: true}
	w := newLifecycleWorker(t, src, resolver)

	w.Start()
	waitFor(t, 2*time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.running
	})

	src.addImgs(model.Image{Id: 1, Url: srv.URL + "/a.png"})
	w.Notify()

	waitFor(t, 2*time.Second, func() bool {
		src.mu.Lock()
		defer src.mu.Unlock()
		return src.updates[1] == [3]int{32, 16, 1}
	})
	waitFor(t, 2*time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.running
	})
}

// TestImageSizeWorkerStopPreventsRestart Shutdown 后 Notify 不得再拉起。
func TestImageSizeWorkerStopPreventsRestart(t *testing.T) {
	var hits int32
	srv := newImgServer(t, map[string][]byte{"/a.png": pngBytes(t, 32, 16)}, &hits)

	resolver := testResolver(srv)
	src := &fakeImageSource{autoDrain: true}
	w := newLifecycleWorker(t, src, resolver)

	w.Start()
	waitFor(t, 2*time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.running
	})

	w.Stop() // Shutdown

	src.addImgs(model.Image{Id: 1, Url: srv.URL + "/a.png"})
	w.Notify() // 停止后不得拉起

	time.Sleep(100 * time.Millisecond) // 若被拉起，多个 tick 间隔内早已解析
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.updates) != 0 {
		t.Errorf("Stop 后 Notify 仍触发了回写 = %v, want 无", src.updates)
	}
}

// TestImageSizeWorkerDisabledConfigDoesNotStart 前端提供的图床配置缺失/错误
// （imgbed.imgUrl 空）→ 不启动，也不拉图回写。
func TestImageSizeWorkerDisabledConfigDoesNotStart(t *testing.T) {
	src := &fakeImageSource{autoDrain: true, imgs: []model.Image{{Id: 1, Url: "http://x/img.png"}}}
	disabled := &ImageSizeResolver{client: newImageSizeClient(""), allowedHost: ""}
	w := newLifecycleWorker(t, src, disabled)

	w.Start()
	w.Notify()
	time.Sleep(100 * time.Millisecond)

	w.mu.Lock()
	running := w.running
	w.mu.Unlock()
	if running {
		t.Error("未配置图床时 worker 仍在运行, want 不启动")
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.updates) != 0 {
		t.Errorf("未配置图床时仍有回写 = %v, want 无", src.updates)
	}
}

func TestHasPendingImage(t *testing.T) {
	if !hasPendingImage([]model.Image{{SizeStatus: model.ImageSizePending}}) {
		t.Error("pending 行未被识别为待解析")
	}
	if hasPendingImage([]model.Image{{SizeStatus: model.ImageSizeOK}, {SizeStatus: model.ImageSizeFailed}}) {
		t.Error("ok/failed 行被误判为待解析")
	}
	if hasPendingImage(nil) {
		t.Error("空集应无待解析")
	}
}

// TestImageSizeWorkerProcessBatchHonorsTickBudget 整轮预算：查询挂死时按 ctx 到期返回，
// 不把 worker 永久卡住（run 内每轮建 60s 预算 ctx，此处直接以 50ms 预算 ctx 验证行为）。
func TestImageSizeWorkerProcessBatchHonorsTickBudget(t *testing.T) {
	src := &fakeImageSource{blockFind: true}
	w := &ImageSizeWorker{source: src, resolver: &ImageSizeResolver{}, l: zap.NewNop()}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	processed := w.processBatch(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("processBatch 未按 tick 预算返回，耗时 %v", elapsed)
	}
	if processed != 0 {
		t.Errorf("processBatch 返回 %d, want 0（查询失败不视为有待解析数据）", processed)
	}
}

// TestImageSizeResolverRejectsOversized 图片头宽高超上限视为异常（损坏/伪造文件），
// 按永久失败处理，超限脏值不回写进库。
func TestImageSizeResolverRejectsOversized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes(t, 25000, 8))
	}))
	t.Cleanup(srv.Close)

	resolver := testResolver(srv)
	_, _, err := resolver.Fetch(context.Background(), srv.URL+"/huge.png")
	if !errors.Is(err, errImagePermanent) {
		t.Errorf("超上限图片 err = %v, want wrap errImagePermanent", err)
	}
}
