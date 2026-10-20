package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/safe"
	"go.uber.org/zap"
)

// imageSizeSource 抽象宽高回填所需的图片行读写，便于单测注入。
type imageSizeSource interface {
	FindZeroSizeImages(ctx context.Context, limit int) ([]model.Image, error)
	UpdateImageSize(ctx context.Context, id int64, width, height int) error
	CopySizeFromDuplicates(ctx context.Context) error
}

var (
	imageSizeDBWriteTimeout = 3 * time.Second
)

const (
	imageSizeBatchSize = 50
	// imageSizeFailMark 解析永久失败行的标记宽高，避免死链每 tick 重试。
	// 前端按未知尺寸兜底；如需重试：UPDATE image SET width=0, height=0 WHERE width < 0。
	imageSizeFailMark = -1
)

// ImageSizeWorker 后台回填 image 表宽高。
// （草稿路径、创建时解析失败的行）：保存时宽高为 0 的行由本 worker 拉取原图解析后回写，
// 存量数据第一轮即被补齐。
type ImageSizeWorker struct {
	source   imageSizeSource
	resolver *ImageSizeResolver
	l        *zap.Logger
	ticker   *time.Ticker
}

func NewImageSizeWorker(source *repo.ImageRepo, resolver *ImageSizeResolver, l *logger.LoggerSet) *ImageSizeWorker {
	w := &ImageSizeWorker{
		source:   source,
		resolver: resolver,
		l:        l.Bff,
		ticker:   time.NewTicker(5 * time.Second),
	}
	if !resolver.Enabled() {
		// 未配置图床域名时禁用拉图（安全默认），已存 0 宽高行保持 0，前端兜底。
		w.l.Warn("imgbed.imgUrl 未配置，宽高回填 worker 不启动")
		return w
	}
	safe.Go(w.l, "image-size-worker", w.run)
	return w
}

func (w *ImageSizeWorker) run() {
	for range w.ticker.C {
		// 每轮单独 recover：单条数据的问题不应让 worker 永久停摆。
		safe.Run(w.l, "image-size-worker.tick", w.processBatch)
	}
}

// processBatch 每 tick 先复制同 URL 已解析的宽高，再拉取剩余 0 宽高行并发解析回写。
func (w *ImageSizeWorker) processBatch() {
	ctx := context.Background()

	// 草稿反复保存会产生同 URL 的大量重复行，先免费复制，减少拉图。
	if err := w.source.CopySizeFromDuplicates(ctx); err != nil {
		w.l.Warn("宽高回填：复制重复行宽高失败", zap.Error(err))
		// 复制失败不致命，继续走拉图路径。
	}

	imgs, err := w.source.FindZeroSizeImages(ctx, imageSizeBatchSize)
	if err != nil {
		w.l.Warn("宽高回填：查询待解析图片失败", zap.Error(err))
		return
	}
	if len(imgs) == 0 {
		return
	}

	sem := make(chan struct{}, imageSizeFetchConcurrent)
	var wg sync.WaitGroup
	for _, img := range imgs {
		wg.Add(1)
		sem <- struct{}{}
		go func(img model.Image) {
			defer wg.Done()
			defer func() { <-sem }()
			// 每条记录独立超时 ctx：拉图可能挂死，不能与整轮 tick 共享 deadline，
			// 否则后续记录的 DB 操作会复用一个已过期的 ctx（同 auditor 送审 worker）。
			fctx, cancel := context.WithTimeout(ctx, imageSizeFetchTimeout)
			defer cancel()

			width, height, err := w.resolver.Fetch(fctx, img.Url)
			dbCtx, dbCancel := context.WithTimeout(context.Background(), imageSizeDBWriteTimeout)
			defer dbCancel()
			if err != nil {
				// 只有永久失败才标 -1 放弃；超时/5xx 等临时失败保持 0，下轮重试。
				if errors.Is(err, errImagePermanent) {
					if uerr := w.source.UpdateImageSize(dbCtx, img.Id, imageSizeFailMark, imageSizeFailMark); uerr != nil {
						w.l.Warn("宽高回填：标记失败行出错", zap.Int64("id", img.Id), zap.Error(uerr))
					}
				}

				w.l.Warn("宽高回填：解析失败", zap.Int64("id", img.Id), zap.String("url", img.Url), zap.Error(err))
				return
			}
			if uerr := w.source.UpdateImageSize(dbCtx, img.Id, width, height); uerr != nil {
				w.l.Warn("宽高回填：回写失败", zap.Int64("id", img.Id), zap.Error(uerr))
			}
		}(img)
	}
	wg.Wait()
}
