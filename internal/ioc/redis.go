package ioc

import (
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/cache"
	"github.com/redis/go-redis/v9"
)

func InitRedis(cfg *config.Conf) *redis.Client {
	addr := cfg.Redis.Addr
	pw := cfg.Redis.Password
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pw,
	})
	return rdb
}

// NewLikeFavoriteRedis 创建点赞收藏 Redis 实例
func NewLikeFavoriteRedis(rdb *redis.Client) *cache.LikeFavoriteRedis {
	return cache.NewLikeFavoriteRedis(rdb)
}
