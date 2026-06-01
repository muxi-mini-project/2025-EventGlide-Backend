package ioc

import (
	"github.com/google/wire"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/mq"
)

var Provider = wire.NewSet(
	// Logger provider
	NewLoggerSet,
	// Other providers
	InitDB,
	InitRedis,
	mq.NewMQ,
	cache.NewCache,
	InitGinHandler,
)