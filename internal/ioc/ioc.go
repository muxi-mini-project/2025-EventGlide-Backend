package ioc

import (
	"github.com/google/wire"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/schedule"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

var Provider = wire.NewSet(
	InitDB,
	InitRedis,
	NewLikeFavoriteRedis,
	mq.NewMQ,
	mq.NewInteractionConsumer,
	NewInteractionSyncTask,
	GetInteractionLogger,
	cache.NewCache,
	InitGinHandler,
)

// NewInteractionSyncTask 创建互动数据同步任务
func NewInteractionSyncTask(lfr *cache.LikeFavoriteRedis, dao *dao.InteractionDao, loggerSet *logger.LoggerSet) *schedule.InteractionSyncTask {
	return schedule.NewInteractionSyncTask(lfr, dao, loggerSet.Interaction)
}

// GetInteractionLogger 获取 interaction zap.Logger
func GetInteractionLogger(loggerSet *logger.LoggerSet) *zap.Logger {
	return loggerSet.Interaction
}
