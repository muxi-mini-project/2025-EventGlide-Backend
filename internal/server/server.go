package server

import (
	"github.com/google/wire"
	"github.com/raiki02/EG/internal/handler"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

var Provider = wire.NewSet(
	NewServer,
)

type Server struct {
	h        *handler.Handler
	l        *zap.Logger
	Shutdown func()
}

func NewServer(h *handler.Handler) *Server {
	return &Server{
		h: h,
		l: logger.GetLogger("bff"),
	}
}

func (s *Server) Run() (err error) {
	s.h.RegisterHandlers()
	err, s.Shutdown = s.h.Run()
	return
}
