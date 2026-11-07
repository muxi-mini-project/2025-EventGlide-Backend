package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// fakeImageSource 内存图片源，记录 worker 的查询与回写，便于断言。
type fakeImageSource struct {
	mu             sync.Mutex
	imgs           []model.Image
	updates        map[int64][3]int // {width, height, size_status}
	updateCnt      int              // UpdateImageSize 调用次数（检测重复回写）
	updateDeadline time.Time        // 最近一次回写收到的 ctx deadline
	copied         bool
	autoDrain      bool // 回写后从待解析集移除该行（模拟 status 变更后的下轮查询）
	blockFind      bool // 阻塞 FindZeroSizeImages 直到 ctx 取消（模拟卡死的查询）
	findErr        error
	panicCopies    int // 前 N 次 CopySizeFromDuplicates 直接 panic（模拟单轮 panic）
	finds          int // FindZeroSizeImages 调用次数
}

func (f *fakeImageSource) FindZeroSizeImages(ctx context.Context, _ int) ([]model.Image, error) {
	f.mu.Lock()
	f.finds++
	findErr := f.findErr
	f.mu.Unlock()
	if f.blockFind {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if findErr != nil {
		return nil, findErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.updateCnt++
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
	if f.panicCopies > 0 {
		f.panicCopies--
		panic("copy size panic")
	}
	f.copied = true
	return nil
}

// addImgs 新增待解析行（模拟创建路径落库后唤醒）。
func (f *fakeImageSource) addImgs(imgs ...model.Image) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.imgs = append(f.imgs, imgs...)
}

func (f *fakeImageSource) findCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.finds
}

func (f *fakeImageSource) setFindErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findErr = err
}

func (f *fakeImageSource) updateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.updateCnt
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

	_, _ = w.processBatch(context.Background())

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

	_, _ = w.processBatch(context.Background())

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

	_, _ = w.processBatch(context.Background())

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

	_, _ = w.processBatch(context.Background())

	src.mu.Lock()
	defer src.mu.Unlock()
	if got := src.updates[1]; got != [3]int{32, 16, 1} {
		t.Fatalf("回写 = %v, want {32 16 1}", got)
	}
	if remain := time.Until(src.updateDeadline); remain < imageSizeDBWriteTimeout/2 {
		t.Errorf("回写 ctx 剩余 %v, 期望独立的 %v 预算；复用拉图 ctx 会导致回写随拉图超时一起失败", remain, imageSizeDBWriteTimeout)
	}
}

// newDecodeServer 第 1 次返回截断 PNG，后续返回完整 PNG；alwaysTruncated 时始终返回截断内容。
func newDecodeServer(t *testing.T, alwaysTruncated bool) (*httptest.Server, *int32) {
	t.Helper()
	full := pngBytes(t, 32, 16)
	truncated := full[:20]
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "image/png")
		if n == 1 || alwaysTruncated {
			_, _ = w.Write(truncated)
			return
		}
		_, _ = w.Write(full)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestImageSizeResolverRetriesDecodeOnce 第一次 decode 失败（截断）时同一次 Fetch 内重试一次，
// 第二次成功即返回宽高，请求次数为 2。
func TestImageSizeResolverRetriesDecodeOnce(t *testing.T) {
	srv, hits := newDecodeServer(t, false)
	resolver := testResolver(srv)

	width, height, err := resolver.Fetch(context.Background(), srv.URL+"/a.png")
	if err != nil {
		t.Fatalf("截断后重试仍失败: %v", err)
	}
	if width != 32 || height != 16 {
		t.Errorf("宽高 = {%d %d}, want {32 16}", width, height)
	}
	if got := atomic.LoadInt32(hits); got != 2 {
		t.Errorf("请求次数 = %d, want 2（decode 失败应重试一次）", got)
	}
}

// TestImageSizeResolverDecodeFailureIsPermanentAfterRetry 两次都 decode 失败才判永久失败。
func TestImageSizeResolverDecodeFailureIsPermanentAfterRetry(t *testing.T) {
	srv, hits := newDecodeServer(t, true)

	_, _, err := testResolver(srv).Fetch(context.Background(), srv.URL+"/a.png")
	if !errors.Is(err, errImagePermanent) {
		t.Fatalf("两次 decode 失败 err = %v, want wrap errImagePermanent", err)
	}
	if got := atomic.LoadInt32(hits); got != 2 {
		t.Errorf("请求次数 = %d, want 2", got)
	}
}

// TestImageSizeResolverServerErrorIsTransientWithoutRetry 5xx 属临时错误，保持 pending 下轮重试，
// 不应在同一次 Fetch 内重试。
func TestImageSizeResolverServerErrorIsTransientWithoutRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, _, err := testResolver(srv).Fetch(context.Background(), srv.URL+"/a.png")
	if err == nil {
		t.Fatal("5xx 应返回错误")
	}
	if errors.Is(err, errImagePermanent) {
		t.Errorf("5xx 被误判为永久失败: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("请求次数 = %d, want 1（5xx 不重试）", got)
	}
}

// TestImageSizeResolverDecodeRetryHonorsCancel 重试期间 ctx 取消：返回 context 错误，不能判永久。
func TestImageSizeResolverDecodeRetryHonorsCancel(t *testing.T) {
	full := pngBytes(t, 32, 16)
	truncated := full[:20]
	secondStarted := make(chan struct{}, 1)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			_, _ = w.Write(truncated)
			return
		}
		secondStarted <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-secondStarted
		cancel()
	}()

	_, _, err := testResolver(srv).Fetch(ctx, srv.URL+"/a.png")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("重试取消 err = %v, want context.Canceled", err)
	}
	if errors.Is(err, errImagePermanent) {
		t.Fatal("ctx 取消被误判为永久失败")
	}
}

// TestImageSizeResolverDecodeCancelDuringRetryIsTransient 第二次读取响应体期间取消时，
// DecodeConfig 只返回 ErrFormat（不含 context.Canceled），不能被判为永久失败。
func TestImageSizeResolverDecodeCancelDuringRetryIsTransient(t *testing.T) {
	full := pngBytes(t, 32, 16)
	truncated := full[:20]
	secondStarted := make(chan struct{}, 1)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			_, _ = w.Write(truncated)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(truncated[:12]) // 头部不完整，DecodeConfig 会继续等后续字节
		w.(http.Flusher).Flush()
		secondStarted <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-secondStarted
		time.Sleep(20 * time.Millisecond) // 让客户端先拿到响应头并进入 DecodeConfig
		cancel()
	}()

	_, _, err := testResolver(srv).Fetch(ctx, srv.URL+"/a.png")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("第二次 decode 期间取消 err = %v, want context.Canceled", err)
	}
	if errors.Is(err, errImagePermanent) {
		t.Fatal("取消被误判为永久失败，行会被写成 size_status=2")
	}
}

// roundTripFunc 用于构造不经过真实网络的 http.Client。
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// errBody 在读完 data 后返回预设的读错误。
type errBody struct {
	data []byte
	err  error
}

func (b *errBody) Read(p []byte) (int, error) {
	if len(b.data) > 0 {
		n := copy(p, b.data)
		b.data = b.data[n:]
		return n, nil
	}
	return 0, b.err
}

func (b *errBody) Close() error { return nil }

// TestImageSizeResolverDecodeReadErrorIsTransient DecodeConfig 在 sniff 阶段可能把
// 连接重置吞成 ErrFormat；底层 net.Error 不能被判为永久失败，也不应重试。
func TestImageSizeResolverDecodeReadErrorIsTransient(t *testing.T) {
	var hits int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&hits, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       &errBody{err: &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}},
			Request:    req,
		}, nil
	})}
	resolver := &ImageSizeResolver{client: client, allowedHost: "img.example.com"}

	_, _, err := resolver.Fetch(context.Background(), "http://img.example.com/a.png")
	if err == nil {
		t.Fatal("读错误应返回 error")
	}
	if errors.Is(err, errImagePermanent) {
		t.Fatalf("网络读错误被误判为永久失败: %v", err)
	}
	var ne net.Error
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want net.Error", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("请求次数 = %d, want 1（网络错误不重试）", got)
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

	w.Start(context.Background())

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

	w.Start(context.Background())

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

	w.Start(context.Background())
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

	w.Start(context.Background())
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

// TestImageSizeWorkerStopCancelsInflightFetch Stop 必须取消在途拉图并等待 run 退出，
// 不能在 Shutdown 后继续跑（回归点：Stop 只 close 信号，在途 Fetch 与 run 都不受控）。
func TestImageSizeWorkerStopCancelsInflightFetch(t *testing.T) {
	entered := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	t.Cleanup(srv.Close)

	src := &fakeImageSource{imgs: []model.Image{{Id: 1, Url: srv.URL + "/slow.png"}}}
	w := newLifecycleWorker(t, src, testResolver(srv))

	w.Start(context.Background())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker 未发起拉图请求")
	}

	start := time.Now()
	w.Stop()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stop 等待 run 退出耗时 %v, want 1s 内", elapsed)
	}

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("在途 Fetch 未收到 cancel")
	}
	w.mu.Lock()
	running := w.running
	w.mu.Unlock()
	if running {
		t.Error("Stop 返回后 run 仍在运行")
	}
}

// TestImageSizeWorkerNotifyStormKeepsSingleRun 解析期间大量并发 Notify 只应保留一个 run，
// 不能出现两个 goroutine 抢同一批行导致重复拉图/回写。
func TestImageSizeWorkerNotifyStormKeepsSingleRun(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes(t, 32, 16))
	}))
	t.Cleanup(srv.Close)

	src := &fakeImageSource{autoDrain: true, imgs: []model.Image{{Id: 1, Url: srv.URL + "/a.png"}}}
	w := newLifecycleWorker(t, src, testResolver(srv))

	w.Start(context.Background())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker 未发起拉图请求")
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Notify()
		}()
	}
	wg.Wait()
	close(release)

	waitFor(t, 2*time.Second, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.running
	})
	if got := src.updateCount(); got != 1 {
		t.Errorf("同一行回写 %d 次, want 1（多个 run 会重复拉图回写）", got)
	}
}

// TestImageSizeWorkerWarnsOnceWhenDisabled 未配置图床时只在首次启动告警，后续 Notify 不重复刷日志。
func TestImageSizeWorkerWarnsOnceWhenDisabled(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	w := &ImageSizeWorker{
		source:   &fakeImageSource{},
		resolver: &ImageSizeResolver{},
		l:        zap.New(core),
		parent:   context.Background(),
	}

	w.Start(context.Background())
	w.Notify()
	w.Notify()

	if got := logs.FilterMessage("imgbed.imgUrl 未配置，宽高回填 worker 不启动").Len(); got != 1 {
		t.Errorf("未配置图床告警 %d 次, want 1", got)
	}
}

// TestImageSizeWorkerOldRunFinishDoesNotClobberNewRun 旧 run 在关闭窗口内退出时，
// 若 Notify 已拉起新 run，其收尾不能把新 run 的 running 清掉（否则下一次 Notify 会再启一个 run）。
func TestImageSizeWorkerOldRunFinishDoesNotClobberNewRun(t *testing.T) {
	w := &ImageSizeWorker{l: zap.NewNop()}

	w.mu.Lock()
	w.running = true
	w.runID = 1
	w.mu.Unlock()

	// 模拟：旧 run 决定关闭并释放锁后，Notify 拉起新 run（runID=2）。
	w.mu.Lock()
	w.runID = 2
	w.running = true
	w.cancel = func() {}
	w.mu.Unlock()

	w.finishRun(1)
	w.mu.Lock()
	running, cancel := w.running, w.cancel
	w.mu.Unlock()
	if !running {
		t.Fatal("旧 run 收尾覆盖了新 run 的 running，下一次 Notify 会再启一个 run")
	}
	if cancel == nil {
		t.Fatal("旧 run 收尾清空了新 run 的 cancel，Stop 将无法取消新 run")
	}

	w.finishRun(2)
	w.mu.Lock()
	running, cancel = w.running, w.cancel
	w.mu.Unlock()
	if running || cancel != nil {
		t.Fatalf("本次 run 收尾后 running=%v cancel!=nil，应为 false/nil", running)
	}
}

// TestImageSizeWorkerDisabledConfigDoesNotStart 前端提供的图床配置缺失/错误
// （imgbed.imgUrl 空）→ 不启动，也不拉图回写。
func TestImageSizeWorkerDisabledConfigDoesNotStart(t *testing.T) {
	src := &fakeImageSource{autoDrain: true, imgs: []model.Image{{Id: 1, Url: "http://x/img.png"}}}
	disabled := &ImageSizeResolver{client: newImageSizeClient(""), allowedHost: ""}
	w := newLifecycleWorker(t, src, disabled)

	w.Start(context.Background())
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

// TestImageSizeWorkerKeepsRunningOnQueryError 查询失败（DB 抖动/ctx 超时）不能被当成“待解析清空”：
// worker 必须保持运行并在下一轮继续查（回归点：返回 0 导致一次抖动就永久停掉回填）。
func TestImageSizeWorkerKeepsRunningOnQueryError(t *testing.T) {
	src := &fakeImageSource{findErr: errors.New("db down")}
	w := newLifecycleWorker(t, src, &ImageSizeResolver{client: newImageSizeClient("img.example.com"), allowedHost: "img.example.com"})

	w.Start(context.Background())
	waitFor(t, 2*time.Second, func() bool { return src.findCount() >= 2 })

	w.mu.Lock()
	running := w.running
	w.mu.Unlock()
	if !running {
		t.Fatal("查询失败后 worker 被关闭, want 保持运行等待下一轮重试")
	}
	w.Stop()
}

// TestImageSizeWorkerSurvivesProcessBatchPanic 单轮 panic 同样不能关闭 worker：
// safe.Run 吞掉 panic 后 ok 保持 false，下一轮继续查询（回归点：processed 初值 0 被误判为清空）。
func TestImageSizeWorkerSurvivesProcessBatchPanic(t *testing.T) {
	src := &fakeImageSource{panicCopies: 1, findErr: errors.New("db down")}
	w := newLifecycleWorker(t, src, &ImageSizeResolver{client: newImageSizeClient("img.example.com"), allowedHost: "img.example.com"})

	w.Start(context.Background())
	waitFor(t, 2*time.Second, func() bool { return src.findCount() >= 1 })

	w.mu.Lock()
	running := w.running
	w.mu.Unlock()
	if !running {
		t.Fatal("单轮 panic 后 worker 被关闭, want 继续运行")
	}
	w.Stop()
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
	processed, ok := w.processBatch(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("processBatch 未按 tick 预算返回，耗时 %v", elapsed)
	}
	if processed != 0 || ok {
		t.Errorf("processBatch 返回 (%d, %v), want (0, false)（查询失败不视为有待解析数据）", processed, ok)
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
