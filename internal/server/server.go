package server

import (
	"context"
	"sync"
	"time"

	"github.com/google/wire"
	"github.com/raiki02/EG/internal/handler"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/schedule"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/safe"
	"go.uber.org/zap"
)

var Provider = wire.NewSet(
	NewServer,
)

type Server struct {
	h           *handler.Handler
	l           *zap.Logger
	worker      *service.AuditorUploadWorker
	imageWorker *service.ImageSizeWorker
	feed        *service.FeedService
	consumer    *mq.InteractionConsumer
	syncTask    *schedule.InteractionSyncTask

	baseShutdown func(context.Context)
	cancel       context.CancelFunc
	shutdownOnce sync.Once
}

func NewServer(h *handler.Handler, worker *service.AuditorUploadWorker, imageWorker *service.ImageSizeWorker, feed *service.FeedService, consumer *mq.InteractionConsumer, syncTask *schedule.InteractionSyncTask) *Server {
	return &Server{
		h:           h,
		l:           logger.GetLogger("bff"),
		worker:      worker,
		imageWorker: imageWorker,
		feed:        feed,
		consumer:    consumer,
		syncTask:    syncTask,
	}
}

func (s *Server) Run() (err error) {
	s.h.RegisterHandlers()
	s.baseShutdown, err = s.h.Run()

	// 后台任务共用同一个可取消 ctx；每个启动调用各自兜底，避免其一 panic 导致其余不被启动。
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	safe.Go(s.l, "interaction-consumer", func() {
		if s.consumer != nil {
			if consumerErr := s.consumer.Start(ctx); consumerErr != nil {
				s.l.Error("Start consumer failed", zap.Error(consumerErr))
			}
		}
	})
	safe.Go(s.l, "interaction-sync-task", func() {
		if s.syncTask != nil {
			s.syncTask.Start(ctx)
		}
	})
	safe.Go(s.l, "feed-consumer", func() {
		if s.feed != nil {
			s.feed.Start(ctx)
		}
	})

	// 宽高回填 worker 按需启动：首轮即查待解析数据，无数据自动关闭，不空转。
	if s.imageWorker != nil {
		s.imageWorker.Start(ctx)
	}
	// 审计送审 worker 由构造时的自动启动改为显式启动。
	if s.worker != nil {
		s.worker.Start(ctx)
	}

	return
}

// serverShutdownBudget 关闭总预算 8s，低于容器 stop grace（默认 10s），留出进程退出的余量：
// HTTP 排空从中最多分 5s，剩余时间给后台任务收尾。
var serverShutdownBudget = 8 * time.Second

// Shutdown 幂等：先停 HTTP 入口，再 cancel 后台 ctx，最后并发 Stop 各组件并在共享 deadline 内等待。
func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), serverShutdownBudget)
		defer cancel()

		if s.baseShutdown != nil {
			s.baseShutdown(ctx)
		}
		if s.cancel != nil {
			s.cancel()
		}

		stops := make([]func(), 0, 5)
		if s.worker != nil {
			stops = append(stops, s.worker.Stop)
		}
		if s.imageWorker != nil {
			stops = append(stops, s.imageWorker.Stop)
		}
		if s.feed != nil {
			stops = append(stops, s.feed.Stop)
		}
		if s.consumer != nil {
			stops = append(stops, s.consumer.Stop)
		}
		if s.syncTask != nil {
			stops = append(stops, s.syncTask.Stop)
		}

		// 各组件共用已取消的父 ctx，退出时间相近；并发 Stop 避免把 5 个 10s 串成 50s。
		var wg sync.WaitGroup
		for _, stop := range stops {
			wg.Add(1)
			go func(stop func()) {
				defer wg.Done()
				stop()
			}(stop)
		}
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			s.l.Warn("后台任务关闭超时", zap.Duration("budget", serverShutdownBudget))
		}
	})
}
