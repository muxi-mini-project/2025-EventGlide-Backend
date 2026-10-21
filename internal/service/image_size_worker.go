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
	UpdateImageSize(ctx context.Context, id int64, width, height, status int) error
	CopySizeFromDuplicates(ctx context.Context) error
}

var (
	imageSizeDBWriteTimeout = 3 * time.Second
	// 回填轮次间隔
	imageSizeTickInterval = 5 * time.Second
	// imageSizeTickTimeout 每轮整轮预算（复制 + 查询 + 拉图），防查询挂死卡住 worker；
	// 单行拉图与回写仍用各自独立 ctx，不受整轮预算影响。
	imageSizeTickTimeout = 60 * time.Second
)

const (
	imageSizeBatchSize = 50
)

// ImageSizeWorker 按需启停的宽高回填 worker（前端为准，只兜底）：
//   - Start 首轮立即执行：无待解析数据（含无存量）立即关闭，不空转；
//   - 每轮解析回写数据库后待解析清空 → 自动关闭；
//   - 创建/草稿落库 pending 行后 Notify 唤醒（运行中标记下轮继续，已关闭则重启）；
//   - Stop（Shutdown）后不再拉起。
type ImageSizeWorker struct {
	source   imageSizeSource
	resolver *ImageSizeResolver
	l        *zap.Logger

	mu      sync.Mutex
	running bool // run goroutine 是否在跑
	wake    bool // 运行期间收到唤醒：本轮结束后继续，防关闭竞态漏解析
	stopCh  chan struct{}
	stopped bool // Shutdown 后不再重启
}

func NewImageSizeWorker(source *repo.ImageRepo, resolver *ImageSizeResolver, l *logger.LoggerSet) *ImageSizeWorker {
	return &ImageSizeWorker{
		source:   source,
		resolver: resolver,
		l:        l.Bff,
	}
}

// Start 启动回填循环（幂等）。已停止、已在运行、图床配置缺失/错误时不启动。
func (w *ImageSizeWorker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.running {
		return
	}
	if !w.resolver.Enabled() {
		w.l.Warn("imgbed.imgUrl 未配置，宽高回填 worker 不启动")
		return
	}

	w.running = true
	w.stopCh = make(chan struct{})
	safe.Go(w.l, "image-size-worker", w.run)
}

// Notify 创建/草稿路径落库 pending 行后唤醒回填：运行中则置 wake（与关闭决策同锁，
// 保证不因关闭竞态漏掉新行），已关闭则重新启动（Start 自带全部守卫）。
func (w *ImageSizeWorker) Notify() {
	w.mu.Lock()
	if w.running {
		w.wake = true
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	w.Start()
}

// Stop 进程关闭时停止回填循环；停止后 Notify 不再拉起。
func (w *ImageSizeWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	if w.stopCh != nil {
		close(w.stopCh)
		w.stopCh = nil
	}
}

func (w *ImageSizeWorker) run() {
	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	ticker := time.NewTicker(imageSizeTickInterval)
	defer ticker.Stop()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), imageSizeTickTimeout)
		processed := 0
		safe.Run(w.l, "image-size-worker.tick", func() {
			processed = w.processBatch(ctx)
		})
		cancel()

		w.mu.Lock()
		if w.wake {
			// 本轮期间有新 pending 落库，立即再跑一轮。
			w.wake = false
			w.mu.Unlock()
			continue
		}

		if processed == 0 {
			// 待解析清空：解析完毕且回写完成，worker 关闭。
			w.running = false
			w.mu.Unlock()
			w.l.Info("宽高回填：待解析数据清空，worker 关闭")
			return
		}
		w.mu.Unlock()

		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
		}
	}
}

// processBatch 每 tick 先复制同 URL 已解析的宽高，再拉取剩余 0 宽高行并发解析回写。
// 返回本轮查到的待解析行数（0 表示清空，调用方可关闭 worker）。
func (w *ImageSizeWorker) processBatch(ctx context.Context) int {
	// 草稿反复保存会产生同 URL 的大量重复行，先免费复制，减少拉图。
	if err := w.source.CopySizeFromDuplicates(ctx); err != nil {
		w.l.Warn("宽高回填：复制重复行宽高失败", zap.Error(err))
		// 复制失败不致命，继续走拉图路径。
	}

	imgs, err := w.source.FindZeroSizeImages(ctx, imageSizeBatchSize)
	if err != nil {
		w.l.Warn("宽高回填：查询待解析图片失败", zap.Error(err))
		return 0
	}
	if len(imgs) == 0 {
		return 0
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
			//回写用独立短超时 ctx：慢图/超时耗尽 fctx 预算后复用会必然失败，
			//行保持 pending 每 tick 死链重试。
			dbCtx, dbCancel := context.WithTimeout(context.Background(), imageSizeDBWriteTimeout)
			defer dbCancel()
			if err != nil {
				// 只有永久失败才置 failed（宽高归零）；超时/5xx 等临时失败保持 pending，下轮重试。
				if errors.Is(err, errImagePermanent) {
					if uerr := w.source.UpdateImageSize(dbCtx, img.Id, 0, 0, model.ImageSizeFailed); uerr != nil {
						w.l.Warn("宽高回填：标记失败行出错", zap.Int64("id", img.Id), zap.Error(uerr))
					}
				}

				w.l.Warn("宽高回填：解析失败", zap.Int64("id", img.Id), zap.String("url", img.Url), zap.Error(err))
				return
			}
			if uerr := w.source.UpdateImageSize(dbCtx, img.Id, width, height, model.ImageSizeOK); uerr != nil {
				w.l.Warn("宽高回填：回写失败", zap.Int64("id", img.Id), zap.Error(uerr))
			}
		}(img)
	}
	wg.Wait()
	return len(imgs)
}

// hasPendingImage 是否含待解析宽高的行（worker 的唤醒条件）。
func hasPendingImage(imgs []model.Image) bool {
	for _, img := range imgs {
		if img.SizeStatus == model.ImageSizePending {
			return true
		}
	}
	return false
}
