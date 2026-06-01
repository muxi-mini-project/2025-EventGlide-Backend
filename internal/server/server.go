package server

import (
	"github.com/google/wire"
	"github.com/raiki02/EG/internal/handler"
	"github.com/raiki02/EG/internal/ioc"
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

func NewServer(h *handler.Handler, ls *ioc.LoggerSet) *Server {
	return &Server{
		h: h,
		l: ls.Bff,
	}
}

func (s *Server) Run() (err error) {
	s.h.RegisterHandlers()
	err, s.Shutdown = s.h.Run()
	return
}
