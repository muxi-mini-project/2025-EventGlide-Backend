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
	// 单行拉图在整轮预算内再各自限时（Stop 时随整轮取消），回写用独立短 ctx 不受影响。
	// 批量 50 行、8 并发、单张最多 10s 时最坏约 62.5s，预算留到 2 分钟避免尾部被取消。
	imageSizeTickTimeout = 2 * time.Minute
	// imageSizeStopTimeout Stop 等待 run 退出的上限，需小于容器 stop grace（默认 10s）。
	imageSizeStopTimeout = 10 * time.Second
)

const (
	imageSizeBatchSize = 50
)

// imageWorkerNotifier 宽高回填唤醒入口，便于创建路径在不依赖具体 worker 实现的情况下单测。
type imageWorkerNotifier interface {
	Notify()
}

// ImageSizeWorker 按需启停的宽高回填 worker（前端为准，只兜底）：
//   - Start 首轮立即执行：无待解析数据（含无存量）立即关闭，不空转；
//   - 每轮解析回写数据库后待解析清空 → 自动关闭；
//   - 创建/草稿落库 pending 行后 Notify 唤醒（运行中标记下轮继续，已关闭则重启）；
//   - Stop（Shutdown）后不再拉起。
type ImageSizeWorker struct {
	source   imageSizeSource
	resolver *ImageSizeResolver
	l        *zap.Logger

	mu             sync.Mutex
	parent         context.Context    // Server 启动时传入，Notify 用它重启 run
	cancel         context.CancelFunc // 当前 run 的 cancel
	running        bool               // run goroutine 是否在跑
	wake           bool               // 运行期间收到唤醒：本轮结束后继续，防关闭竞态漏解析
	stopped        bool               // Shutdown 后不再重启
	runID          uint64             // run 序号，防止旧 run 的收尾覆盖新 run 的状态
	disabledWarned bool               // 未配置图床只告警一次
	wg             sync.WaitGroup     // 等待 run 退出
}

func NewImageSizeWorker(source *repo.ImageRepo, resolver *ImageSizeResolver, l *logger.LoggerSet) *ImageSizeWorker {
	return &ImageSizeWorker{
		source:   source,
		resolver: resolver,
		l:        l.Bff,
		parent:   context.Background(),
	}
}

// Start 启动回填循环（幂等）。已停止、已在运行、图床配置缺失/错误时不启动。
func (w *ImageSizeWorker) Start(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.running {
		return
	}
	w.parent = ctx
	if !w.resolver.Enabled() {
		if !w.disabledWarned {
			w.disabledWarned = true
			w.l.Warn("imgbed.imgUrl 未配置，宽高回填 worker 不启动")
		}
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.running = true
	w.runID++
	id := w.runID
	w.wg.Add(1)
	safe.Go(w.l, "image-size-worker", func() {
		defer w.wg.Done()
		w.run(runCtx, id)
	})
}

// Notify 创建/草稿路径落库 pending 行后唤醒回填：运行中则置 wake（与关闭决策同锁，
// 保证不因关闭竞态漏掉新行），已关闭则用 Server 启动时的 parent ctx 重新拉起。
func (w *ImageSizeWorker) Notify() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	if w.running {
		w.wake = true
		w.mu.Unlock()
		return
	}
	parent := w.parent
	w.mu.Unlock()
	w.Start(parent)
}

// Stop 进程关闭时停止回填循环：取消在途请求并有界等待 run 退出；停止后 Notify 不再拉起。
func (w *ImageSizeWorker) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	cancel := w.cancel
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(imageSizeStopTimeout):
		w.l.Warn("宽高回填：停止超时", zap.Duration("timeout", imageSizeStopTimeout))
	}
}

func (w *ImageSizeWorker) run(ctx context.Context, id uint64) {
	defer w.finishRun(id)

	ticker := time.NewTicker(imageSizeTickInterval)
	defer ticker.Stop()

	for {
		roundCtx, cancel := context.WithTimeout(ctx, imageSizeTickTimeout)
		processed, ok := 0, false
		safe.Run(w.l, "image-size-worker.tick", func() {
			processed, ok = w.processBatch(roundCtx)
		})
		cancel()

		w.mu.Lock()
		if w.runID != id {
			w.mu.Unlock()
			return
		}
		if w.wake {
			// 本轮期间有新 pending 落库，立即再跑一轮。
			w.wake = false
			w.mu.Unlock()
			continue
		}

		if ok && processed == 0 {
			// 待解析清空：解析完毕且回写完成，worker 关闭。
			// 查询失败、ctx 超时或 panic（ok=false）不关闭，留待下一轮重试。
			w.running = false
			w.cancel = nil
			w.mu.Unlock()
			w.l.Info("宽高回填：待解析数据清空，worker 关闭")
			return
		}
		w.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// finishRun 收尾本次 run：只在仍是本次 run 时清理状态，
// 关闭窗口内 Notify 已拉起新 run 时不能覆盖它的 running/cancel。
func (w *ImageSizeWorker) finishRun(id uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.runID == id {
		w.running = false
		w.cancel = nil
	}
}

// processBatch 每 tick 先复制同 URL 已解析的宽高，再拉取剩余 0 宽高行并发解析回写。
// 返回本轮查到的待解析行数，以及本轮是否成功完成查询：
//   - (0, false) 查询失败/ctx 超时（含 panic 未赋值）：调用方不得视为“已清空”；
//   - (0, true)  查询成功且无待解析行：调用方可关闭 worker；
//   - (n, true)  本轮查到 n 行（含拉取失败、保持 pending 的行）。
func (w *ImageSizeWorker) processBatch(ctx context.Context) (int, bool) {
	// 草稿反复保存会产生同 URL 的大量重复行，先免费复制，减少拉图。
	if err := w.source.CopySizeFromDuplicates(ctx); err != nil {
		w.l.Warn("宽高回填：复制重复行宽高失败", zap.Error(err))
		// 复制失败不致命，继续走拉图路径。
	}

	imgs, err := w.source.FindZeroSizeImages(ctx, imageSizeBatchSize)
	if err != nil {
		w.l.Warn("宽高回填：查询待解析图片失败", zap.Error(err))
		return 0, false
	}
	if len(imgs) == 0 {
		return 0, true
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
	return len(imgs), true
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
