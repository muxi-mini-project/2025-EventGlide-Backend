package service

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/raiki02/EG/config"
)

var (
	// imageSizeFetchTimeout 单张图片拉取超时。
	imageSizeFetchTimeout = 10 * time.Second
	// url为非可解析宽高内容(4xx、非白名单、跨域重定向、非图片)
	errImagePermanent = errors.New("image permanently unresolvable")
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
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", errImagePermanent, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return 0, 0, fmt.Errorf("%w: scheme %q 不支持", errImagePermanent, u.Scheme)
	}
	if !strings.EqualFold(u.Host, r.allowedHost) {
		return 0, 0, fmt.Errorf("%w: host %q 不在允许列表（%s）", errImagePermanent, u.Host, r.allowedHost)
	}

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
	cfg, _, err := image.DecodeConfig(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: 图片解析格式失败; %v", errImagePermanent, err)
	}
	return cfg.Width, cfg.Height, nil
}

// hostFromURL 提取图床域名的小写 host（含端口），空串表示未配置。
func hostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}
