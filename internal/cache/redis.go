package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client redis.UniversalClient
	codec  Codec
}

func NewRedisCache(client redis.UniversalClient, codec Codec) *RedisCache {
	if codec == nil {
		codec = JSONCodec{}
	}
	return &RedisCache{
		client: client,
		codec:  codec,
	}
}

func (c *RedisCache) Get(ctx context.Context, key string) (any, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	var item cacheItem
	if err = json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("cache: redis ttl must be positive")
	}

	var (
		item cacheItem
		err  error
	)
	switch v := value.(type) {
	case *cacheItem:
		item = *v
	case cacheItem:
		item = v
	default:
		item.Payload, err = c.codec.Marshal(value)
		if err != nil {
			return err
		}
	}

	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, raw, ttl).Err()
}

func (c *RedisCache) Del(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
