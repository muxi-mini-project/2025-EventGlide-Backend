//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/handler"
	"github.com/raiki02/EG/internal/ioc"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/internal/server"
	"github.com/raiki02/EG/internal/service"
)

func InitApp() *server.Server {
	panic(wire.Build(
		config.InitConf,
		ioc.Provider,
		middleware.Provider,
		dao.Provider,
		repo.Provider,
		handler.Provider,
		service.Provider,
		server.Provider,
	))
}