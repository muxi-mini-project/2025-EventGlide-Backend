package server

import (
	"context"

	"github.com/google/wire"
	"github.com/raiki02/EG/internal/handler"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/schedule"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

var Provider = wire.NewSet(
	NewServer,
)

type Server struct {
	h        *handler.Handler
	l        *zap.Logger
	worker   *service.AuditorUploadWorker
	consumer *mq.InteractionConsumer
	syncTask *schedule.InteractionSyncTask
	Shutdown func()
}

func NewServer(h *handler.Handler, worker *service.AuditorUploadWorker, consumer *mq.InteractionConsumer, syncTask *schedule.InteractionSyncTask) *Server {
	return &Server{
		h:        h,
		l:        logger.GetLogger("bff"),
		worker:   worker,
		consumer: consumer,
		syncTask: syncTask,
	}
}

func (s *Server) Run() (err error) {
	s.h.RegisterHandlers()
	err, baseShutdown := s.h.Run()

	// 启动后台任务
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if s.consumer != nil {
			if consumerErr := s.consumer.Start(ctx); consumerErr != nil {
				s.l.Error("Start consumer failed", zap.Error(consumerErr))
			}
		}
		if s.syncTask != nil {
			s.syncTask.Start(ctx)
		}
	}()

	s.Shutdown = func() {
		baseShutdown()
		cancel()
	}
	return
}
