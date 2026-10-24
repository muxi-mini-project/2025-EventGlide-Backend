package service

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
)

var (
	// imageSizeFetchTimeout 单张图片拉取超时。
	imageSizeFetchTimeout = 10 * time.Second
	// url为非可解析宽高内容(4xx、非白名单、跨域重定向、非图片)
	errImagePermanent = errors.New("image permanently unresolvable")
	// errImageDecode 图片头解析失败：可能是不支持的格式，也可能是响应被截断；
	// 同一次 Fetch 内重试一次，仍失败才按永久失败处理。
	errImageDecode = errors.New("image decode failed")
)

const (
	// imageSizeFetchConcurrent 拉图解析并发上限（网络 IO 密集）。
	imageSizeFetchConcurrent = 8
)

// ImageSizeResolver 拉取图片并解析宽高的共享实现，只服务回填 worker（前端为准，创建路径不拉图）。
// 只允许拉取图床域名下的 URL：showImg 由客户端提交，任意 URL 都可入库，不能盲拉（防 SSRF）。
// 支持 jpg/png/gif/webp；heic 等其他格式解析失败保持 0 交前端兜底。
type ImageSizeResolver struct {
	client      *http.Client
	allowedHost string
}

func NewImageSizeResolver(cfg *config.Conf) *ImageSizeResolver {
	allowedHost := hostFromURL(cfg.Imgbed.ImgURL)
	return &ImageSizeResolver{
		client:      newImageSizeClient(allowedHost),
		allowedHost: allowedHost,
	}
}

func newImageSizeClient(allowedHost string) *http.Client {
	return &http.Client{
		Timeout: imageSizeFetchTimeout,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if !strings.EqualFold(req.URL.Host, allowedHost) {
				return fmt.Errorf("%w: redirect host %q 不在允许列表（%s）", errImagePermanent, req.URL.Host, allowedHost)
			}

			return nil
		},
	}
}

// Enabled 图床域名已配置才允许拉图。
func (r *ImageSizeResolver) Enabled() bool { return r.allowedHost != "" }

// Fetch 拉取单张图片解析宽高。
func (r *ImageSizeResolver) Fetch(ctx context.Context, rawURL string) (int, int, error) {
	if err := r.validateURL(rawURL); err != nil {
		return 0, 0, err
	}

	width, height, err := r.requestSize(ctx, rawURL)
	if err != nil && errors.Is(err, errImageDecode) {
		// 取消/超时期间 DecodeConfig 可能只返回 ErrFormat，先按临时失败返回，且不再重试。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, 0, ctxErr
		}
		// 可能是截断/半张图，重试一次；真不支持的格式会在第二次后判永久。
		width, height, err = r.requestSize(ctx, rawURL)
	}
	if err != nil && errors.Is(err, errImageDecode) {
		// 第二次读取响应体期间被取消/超时，同样不能判永久，留给下一轮重试。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, 0, ctxErr
		}
		return 0, 0, fmt.Errorf("%w: %v", errImagePermanent, err)
	}
	return width, height, err
}

// validateURL 只允许拉取图床域名下的 http(s) URL（防 SSRF）。
func (r *ImageSizeResolver) validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", errImagePermanent, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q 不支持", errImagePermanent, u.Scheme)
	}
	if !strings.EqualFold(u.Host, r.allowedHost) {
		return fmt.Errorf("%w: host %q 不在允许列表（%s）", errImagePermanent, u.Host, r.allowedHost)
	}
	return nil
}

// requestSize 执行单次「请求 + 状态码 + 解码 + 上限检查」。
// decode 失败返回 errImageDecode（可重试），其余错误分类保持不变。
func (r *ImageSizeResolver) requestSize(ctx context.Context, rawURL string) (int, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", errImagePermanent, err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout:
		return 0, 0, fmt.Errorf("拉取图片返回: %s", resp.Status)
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return 0, 0, fmt.Errorf("%w: 拉取图片返回 %s", errImagePermanent, resp.Status)
	case resp.StatusCode != http.StatusOK:
		return 0, 0, fmt.Errorf("拉取图片返回 %s", resp.Status)
	}
	rec := &readErrorRecorder{r: resp.Body}
	cfg, _, err := image.DecodeConfig(rec)
	if err != nil {
		// DecodeConfig 可能把读错误吞成 ErrFormat（sniff 的 Peek 失败），
		// 用记录到的底层错误区分网络中断/取消与真正的格式错误。
		if terr := rec.transientErr(); terr != nil {
			return 0, 0, fmt.Errorf("读取图片失败: %w", terr)
		}
		return 0, 0, fmt.Errorf("%w: %v", errImageDecode, err)
	}
	// 图片头宽高超上限视为异常（损坏/伪造文件），按永久失败处理，脏值不回写。
	if cfg.Width > model.ImageSizeMaxEdge || cfg.Height > model.ImageSizeMaxEdge {
		return 0, 0, fmt.Errorf("%w: 图片宽高超上限 %dx%d", errImagePermanent, cfg.Width, cfg.Height)
	}
	return cfg.Width, cfg.Height, nil
}

// readErrorRecorder 记录响应体读取的底层错误：image.DecodeConfig 在 sniff 阶段
// 会把读错误吞成 ErrFormat，无法区分网络中断、取消与真正的格式不支持。
type readErrorRecorder struct {
	r   io.Reader
	err error
}

func (r *readErrorRecorder) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return n, err
}

// transientErr 底层读错误属于网络/取消类时返回它，供调用方保持 pending 重试；
// 普通格式错误或干净截断（ErrUnexpectedEOF）返回 nil，仍走 decode 重试。
func (r *readErrorRecorder) transientErr() error {
	if r.err == nil {
		return nil
	}
	var ne net.Error
	if errors.As(r.err, &ne) ||
		errors.Is(r.err, context.Canceled) ||
		errors.Is(r.err, context.DeadlineExceeded) {
		return r.err
	}
	return nil
}

// hostFromURL 提取图床域名的小写 host（含端口），空串表示未配置。
func hostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}
