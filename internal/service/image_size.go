package service

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
)

const (
	// imageSizeFetchTimeout 单张图片拉取超时。
	imageSizeFetchTimeout = 10 * time.Second
	// imageSizeFetchConcurrent 拉图解析并发上限（网络 IO 密集）。
	imageSizeFetchConcurrent = 8
	// imageSizeResolveBudget 创建路径同步解析的整批预算：拉图是尽力而为，
	// 预算内未解析的行保持 0，交由回填 worker 异步重试。
	imageSizeResolveBudget = 5 * time.Second
)

// ImageSizeResolver 拉取图片并解析宽高的共享实现：
// 创建路径用 ResolveMissing 同步兜底（创建后前端立即刷新列表也要能拿到宽高），
// 回填 worker 用 Fetch 异步兜底（存量、草稿与创建时解析失败的行）。
// 只允许拉取图床域名下的 URL：showImg 由客户端提交，任意 URL 都可入库，不能盲拉（防 SSRF）。
// 注意：std 库不认 webp，webp 图解析失败保持 0；如需支持可引入 golang.org/x/image/webp。
type ImageSizeResolver struct {
	client      *http.Client
	allowedHost string
}

func NewImageSizeResolver(cfg *config.Conf) *ImageSizeResolver {
	return &ImageSizeResolver{
		client:      &http.Client{Timeout: imageSizeFetchTimeout},
		allowedHost: hostFromURL(cfg.Imgbed.ImgURL),
	}
}

// Enabled 图床域名已配置才允许拉图。
func (r *ImageSizeResolver) Enabled() bool { return r.allowedHost != "" }

// Fetch 拉取单张图片解析宽高。
func (r *ImageSizeResolver) Fetch(ctx context.Context, rawURL string) (int, int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, 0, err
	}
	if !strings.EqualFold(u.Host, r.allowedHost) {
		return 0, 0, fmt.Errorf("host %q 不在允许列表（%s）", u.Host, r.allowedHost)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, 0, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("拉取图片返回 %s", resp.Status)
	}
	cfg, _, err := image.DecodeConfig(resp.Body)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// ResolveMissing 并发为宽高为 0 的图片解析宽高（原地填充）。
// 请求里已带宽高的行直接跳过；解析失败保持 0，交由回填 worker 重试。
func (r *ImageSizeResolver) ResolveMissing(ctx context.Context, imgs []model.Image) {
	if !r.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, imageSizeResolveBudget)
	defer cancel()

	sem := make(chan struct{}, imageSizeFetchConcurrent)
	var wg sync.WaitGroup
	for i := range imgs {
		if imgs[i].Width != 0 || imgs[i].Height != 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(img *model.Image) {
			defer wg.Done()
			defer func() { <-sem }()
			width, height, err := r.Fetch(ctx, img.Url)
			if err != nil {
				return
			}
			img.Width, img.Height = width, height
		}(&imgs[i])
	}
	wg.Wait()
}

// hostFromURL 提取图床域名的小写 host（含端口），空串表示未配置。
func hostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}
