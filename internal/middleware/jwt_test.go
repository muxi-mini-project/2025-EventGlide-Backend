package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/raiki02/EG/config"
	"github.com/redis/go-redis/v9"
)

func newTestJwt() (*Jwt, *miniredis.Miniredis, *redis.Client, error) {
	s, err := miniredis.Run()
	if err != nil {
		return nil, nil, nil, err
	}
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	cfg := &config.Conf{}
	cfg.JWT.Key = "test-jwt-key"
	cfg.JWT.Ttl = 259200
	return NewJwt(rdb, cfg), s, rdb, nil
}

func TestCheckTokenSlidingExpiry(t *testing.T) {
	j, s, rdb, err := newTestJwt()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defer rdb.Close()

	ctx := context.Background()
	token := j.GenToken(ctx, "2021000001")
	if err := j.StoreInRedis(ctx, "2021000001", token); err != nil {
		t.Fatal(err)
	}
	id := j.parseTokenId(token)
	key := "token:" + id

	// 模拟 token 快过期（TTL 10 秒）
	if err := rdb.Expire(ctx, key, 10*time.Second).Err(); err != nil {
		t.Fatal(err)
	}

	if err := j.CheckToken(ctx, token); err != nil {
		t.Fatalf("CheckToken error: %v", err)
	}

	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl < 259200*time.Second {
		t.Fatalf("CheckToken 未续期: TTL = %v, 期望重置为 ~3 天", ttl)
	}
}

func TestCheckTokenExpired(t *testing.T) {
	j, s, rdb, err := newTestJwt()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defer rdb.Close()

	ctx := context.Background()
	token := j.GenToken(ctx, "2021000001")
	if err := j.StoreInRedis(ctx, "2021000001", token); err != nil {
		t.Fatal(err)
	}

	// 删除 key 模拟过期
	id := j.parseTokenId(token)
	if err := rdb.Del(ctx, "token:"+id).Err(); err != nil {
		t.Fatal(err)
	}

	if err := j.CheckToken(ctx, token); !errors.Is(err, redis.Nil) {
		t.Fatalf("CheckToken 应因 key 不存在返回过期错误（底层 redis.Nil），got: %v", err)
	}
}
