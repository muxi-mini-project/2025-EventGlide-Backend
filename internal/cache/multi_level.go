package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type MultiLevelCache struct {
	local  Cache
	remote Cache
	ops    options
	sf     singleflight.Group
	randMu sync.Mutex
}

func NewMultiLevelCache(local Cache, remote Cache, opts ...Option) *MultiLevelCache {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	return &MultiLevelCache{
		local:  local,
		remote: remote,
		ops:    options,
	}
}

func NewCache(rdb *redis.Client) *MultiLevelCache {
	local, err := NewLocalCache()
	if err != nil {
		panic(fmt.Errorf("init local cache: %w", err))
	}
	return NewMultiLevelCache(local, NewRedisCache(rdb, JSONCodec{}))
}

func (c *MultiLevelCache) Get(
	ctx context.Context,
	key string,
	ttl time.Duration,
	loader func(context.Context) (any, error),
) (any, error) {
	return c.get(ctx, key, ttl, func(data []byte) (any, error) {
		var target any
		if err := c.ops.codec.Unmarshal(data, &target); err != nil {
			return nil, err
		}
		return target, nil
	}, loader)
}

func GetTyped[T any](
	c *MultiLevelCache,
	ctx context.Context,
	key string,
	ttl time.Duration,
	loader func(context.Context) (T, error),
) (T, error) {
	var zero T
	res, err := c.get(ctx, key, ttl, func(data []byte) (any, error) {
		var target T
		if decodeErr := c.ops.codec.Unmarshal(data, &target); decodeErr != nil {
			return nil, decodeErr
		}
		return target, nil
	}, func(ctx context.Context) (any, error) {
		return loader(ctx)
	})
	if err != nil {
		return zero, err
	}

	value, ok := res.(T)
	if ok {
		return value, nil
	}
	if res == nil {
		return zero, nil
	}
	return zero, fmt.Errorf("cache: unexpected value type %T", res)
}

func (c *MultiLevelCache) get(
	ctx context.Context,
	key string,
	ttl time.Duration,
	decode func([]byte) (any, error),
	loader func(context.Context) (any, error),
) (any, error) {
	if value, err := c.readLocal(ctx, key); err == nil {
		return value, nil
	} else if !c.isLocalCachedNotFoundError(err) && !c.isCacheMissError(err) {
		return nil, err
	}

	if value, err := c.readRemote(ctx, key, ttl, decode); err == nil {
		return value, nil
	} else if !c.isRemoteCachedNotFoundError(err) && !c.isCacheMissError(err) {
		return nil, ErrNotFound
	}

	res, err, _ := c.sf.Do(key, func() (any, error) {
		if value, readErr := c.readLocal(ctx, key); readErr == nil {
			return value, nil
		} else if !c.isLocalCachedNotFoundError(readErr) && !c.isCacheMissError(readErr) {
			return nil, readErr
		}

		if value, readErr := c.readRemote(ctx, key, ttl, decode); readErr == nil {
			return value, nil
		} else if !c.isRemoteCachedNotFoundError(readErr) && !c.isCacheMissError(readErr) {
			return nil, ErrNotFound
		}

		start := time.Now()
		value, loadErr := loader(ctx)
		c.ops.metrics.ObserveLoad(ctx, key, time.Since(start), loadErr)
		if loadErr != nil {
			if c.ops.notFound(loadErr) {
				cacheErr := c.writeNullCaches(ctx, key)
				if cacheErr != nil {
					loadErr = errors.Join(loadErr, cacheErr)
				}
			}
			return nil, loadErr
		}

		if writeErr := c.writeCaches(ctx, key, value, ttl); writeErr != nil {
			return value, writeErr
		}
		return value, nil
	})
	return res, err
}

func (c *MultiLevelCache) SetAndInvalidate(
	ctx context.Context,
	key string,
	value any,
	ttl time.Duration,
) error {
	var err error
	if c.remote != nil {
		err = errors.Join(err, c.remote.Del(ctx, key))
	}
	if c.local != nil {
		err = errors.Join(err, c.local.Del(ctx, key))
	}
	if c.ops.hook != nil {
		err = errors.Join(err, c.ops.hook.AfterInvalidate(ctx, key, value, ttl))
	}
	return err
}

func (c *MultiLevelCache) readLocal(ctx context.Context, key string) (any, error) {
	if c.local == nil {
		return nil, ErrCacheMiss
	}

	value, err := c.local.Get(ctx, key)
	if err != nil {
		c.ops.metrics.ObserveLocalMiss(ctx, key)
		return nil, ErrCacheMiss
	}

	entry, ok := value.(*localEntry)
	if !ok {
		c.ops.metrics.ObserveLocalMiss(ctx, key)
		return nil, ErrCacheMiss
	}
	if entry.Nil {
		c.ops.metrics.ObserveLocalHit(ctx, key)
		return nil, ErrLocalCachedNotFound
	}

	c.ops.metrics.ObserveLocalHit(ctx, key)
	return entry.Value, nil
}

func (c *MultiLevelCache) readRemote(
	ctx context.Context,
	key string,
	ttl time.Duration,
	decode func([]byte) (any, error),
) (any, error) {
	if c.remote == nil {
		return nil, ErrCacheMiss
	}

	value, err := c.remote.Get(ctx, key)
	if err != nil {
		c.ops.metrics.ObserveRemoteMiss(ctx, key)
		return nil, ErrCacheMiss
	}

	item, ok := value.(*cacheItem)
	if !ok {
		c.ops.metrics.ObserveRemoteMiss(ctx, key)
		return nil, ErrCacheMiss
	}
	if item.Nil {
		c.ops.metrics.ObserveRemoteHit(ctx, key)
		c.writeLocalSilently(ctx, key, &localEntry{Nil: true}, c.emptyLocalTTL())
		return nil, ErrRemoteCachedNotFound
	}

	target, err := decode(item.Payload)
	if err != nil {
		c.ops.metrics.ObserveRemoteMiss(ctx, key)
		return nil, ErrCacheMiss
	}

	c.ops.metrics.ObserveRemoteHit(ctx, key)
	c.writeLocalSilently(ctx, key, &localEntry{Value: target}, c.localTTL(ttl))
	return target, nil
}

func (c *MultiLevelCache) writeCaches(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("cache: ttl must be positive")
	}

	var err error
	if c.remote != nil {
		err = c.remote.Set(ctx, key, value, c.remoteTTL(ttl))
	}
	c.writeLocalSilently(ctx, key, &localEntry{Value: value}, c.localTTL(ttl))
	return err
}

func (c *MultiLevelCache) writeNullCaches(ctx context.Context, key string) error {
	item := &cacheItem{Nil: true}
	var err error
	if c.remote != nil {
		err = c.remote.Set(ctx, key, item, c.emptyRemoteTTL())
	}
	c.writeLocalSilently(ctx, key, &localEntry{Nil: true}, c.emptyLocalTTL())
	return err
}

func (c *MultiLevelCache) writeLocalSilently(ctx context.Context, key string, value any, ttl time.Duration) {
	if c.local == nil || ttl <= 0 {
		return
	}
	_ = c.local.Set(ctx, key, value, ttl)
}

func (c *MultiLevelCache) remoteTTL(base time.Duration) time.Duration {
	return base + c.jitter()
}

func (c *MultiLevelCache) localTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	localTTL := time.Duration(float64(base) * c.ops.localTTLRatio)
	if localTTL <= 0 {
		localTTL = time.Millisecond
	}
	jitter := c.jitter()
	if localTTL+jitter >= base {
		jitter = 0
	}
	return localTTL + jitter
}

func (c *MultiLevelCache) emptyRemoteTTL() time.Duration {
	return c.remoteTTL(c.ops.emptyTTL)
}

func (c *MultiLevelCache) emptyLocalTTL() time.Duration {
	return c.localTTL(c.ops.emptyTTL)
}

func (c *MultiLevelCache) jitter() time.Duration {
	if c.ops.jitterMax <= 0 || c.ops.randSource == nil {
		return 0
	}
	c.randMu.Lock()
	defer c.randMu.Unlock()
	return time.Duration(c.ops.randSource.Int63n(int64(c.ops.jitterMax) + 1))
}

func (c *MultiLevelCache) isLocalCachedNotFoundError(err error) bool {
	return errors.Is(err, ErrLocalCachedNotFound)
}

func (c *MultiLevelCache) isRemoteCachedNotFoundError(err error) bool {
	return errors.Is(err, ErrRemoteCachedNotFound)
}

func (c *MultiLevelCache) isCacheMissError(err error) bool {
	return errors.Is(err, ErrCacheMiss)
}
