package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand"
	"time"
)

var (
	ErrCacheMiss            = errors.New("cache: miss")
	ErrLocalCachedNotFound  = errors.New("cache: local cached not found")
	ErrRemoteCachedNotFound = errors.New("cache: remote cached not found")
	ErrNotFound             = errors.New("cache: not found")
)

type Cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

type Codec interface {
	Marshal(value any) ([]byte, error)
	Unmarshal(data []byte, dst any) error
}

type JSONCodec struct{}

func (JSONCodec) Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (JSONCodec) Unmarshal(data []byte, dst any) error {
	return json.Unmarshal(data, dst)
}

type Metrics interface {
	ObserveLocalHit(ctx context.Context, key string)
	ObserveLocalMiss(ctx context.Context, key string)
	ObserveRemoteHit(ctx context.Context, key string)
	ObserveRemoteMiss(ctx context.Context, key string)
	ObserveLoad(ctx context.Context, key string, d time.Duration, err error)
}

type NoopMetrics struct{}

func (NoopMetrics) ObserveLocalHit(context.Context, string)                   {}
func (NoopMetrics) ObserveLocalMiss(context.Context, string)                  {}
func (NoopMetrics) ObserveRemoteHit(context.Context, string)                  {}
func (NoopMetrics) ObserveRemoteMiss(context.Context, string)                 {}
func (NoopMetrics) ObserveLoad(context.Context, string, time.Duration, error) {}

type InvalidationHook interface {
	AfterInvalidate(ctx context.Context, key string, value any, ttl time.Duration) error
}

type NoopInvalidationHook struct{}

func (NoopInvalidationHook) AfterInvalidate(context.Context, string, any, time.Duration) error {
	return nil
}

type Option func(*options)

type options struct {
	codec         Codec
	metrics       Metrics
	hook          InvalidationHook
	notFound      func(error) bool
	localTTLRatio float64
	jitterMax     time.Duration
	emptyTTL      time.Duration
	randSource    *rand.Rand
}

func defaultOptions() options {
	return options{
		codec:         JSONCodec{},
		metrics:       NoopMetrics{},
		hook:          NoopInvalidationHook{},
		notFound:      IsNotFound,
		localTTLRatio: 0.8,
		jitterMax:     5 * time.Second,
		emptyTTL:      30 * time.Second,
		randSource:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func WithCodec(codec Codec) Option {
	return func(o *options) {
		if codec != nil {
			o.codec = codec
		}
	}
}

func WithMetrics(metrics Metrics) Option {
	return func(o *options) {
		if metrics != nil {
			o.metrics = metrics
		}
	}
}

func WithInvalidationHook(hook InvalidationHook) Option {
	return func(o *options) {
		if hook != nil {
			o.hook = hook
		}
	}
}

func WithNotFoundMatcher(fn func(error) bool) Option {
	return func(o *options) {
		if fn != nil {
			o.notFound = fn
		}
	}
}

func WithLocalTTLRatio(ratio float64) Option {
	return func(o *options) {
		if ratio > 0 && ratio < 1 {
			o.localTTLRatio = ratio
		}
	}
}

func WithJitter(max time.Duration) Option {
	return func(o *options) {
		if max >= 0 {
			o.jitterMax = max
		}
	}
}

func WithEmptyTTL(ttl time.Duration) Option {
	return func(o *options) {
		if ttl > 0 {
			o.emptyTTL = ttl
		}
	}
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows)
}

func MarkNotFound(err error) error {
	if err == nil {
		return ErrNotFound
	}
	return errors.Join(ErrNotFound, err)
}

type KeyBuilder struct {
	namespace string
}

func NewKeyBuilder(namespace string) KeyBuilder {
	return KeyBuilder{namespace: namespace}
}

func (b KeyBuilder) Build(parts ...string) string {
	if b.namespace == "" && len(parts) == 0 {
		return ""
	}
	key := b.namespace
	for _, part := range parts {
		if key == "" {
			key = part
			continue
		}
		key += ":" + part
	}
	return key
}

type cacheItem struct {
	Nil     bool            `json:"nil"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type localEntry struct {
	Nil   bool
	Value any
}
