package cache

import (
	"context"
	"errors"
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

type LocalCache struct {
	cache *ristretto.Cache[string, any]
}

func NewLocalCache() (*LocalCache, error) {
	rc, err := ristretto.NewCache(&ristretto.Config[string, any]{
		NumCounters: 1 << 20,
		MaxCost:     1 << 28,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &LocalCache{cache: rc}, nil
}

func NewLocalCacheWithConfig(cfg *ristretto.Config[string, any]) (*LocalCache, error) {
	rc, err := ristretto.NewCache(cfg)
	if err != nil {
		return nil, err
	}
	return &LocalCache{cache: rc}, nil
}

func (c *LocalCache) Get(_ context.Context, key string) (any, error) {
	value, ok := c.cache.Get(key)
	if !ok {
		return nil, ErrCacheMiss
	}
	return value, nil
}

func (c *LocalCache) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("cache: local ttl must be positive")
	}
	c.cache.SetWithTTL(key, value, 1, ttl)
	return nil
}

func (c *LocalCache) Del(_ context.Context, key string) error {
	c.cache.Del(key)
	return nil
}

func (c *LocalCache) Close() error {
	c.cache.Close()
	return nil
}
