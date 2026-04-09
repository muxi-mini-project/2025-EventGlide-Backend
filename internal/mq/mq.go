package mq

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type MQHdl interface {
	Publish(ctx context.Context, stream string, message interface{}) error
	EnsureConsumerGroup(ctx context.Context, stream, group string) error
	ConsumeGroup(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]redis.XMessage, error)
	Ack(ctx context.Context, stream, group string, ids ...string) error
}

type MQ struct {
	rdb *redis.Client
}

func NewMQ(rdb *redis.Client) MQHdl {
	mq := &MQ{
		rdb: rdb,
	}
	return mq
}

func (mq *MQ) Publish(ctx context.Context, stream string, message interface{}) error {
	jsonReq, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return mq.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"data": jsonReq,
		},
		MaxLen: 10000,
		Approx: true,
	}).Err()
}

func (mq *MQ) EnsureConsumerGroup(ctx context.Context, stream, group string) error {
	err := mq.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

func (mq *MQ) ConsumeGroup(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]redis.XMessage, error) {
	res, err := mq.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()

	if err != nil && errors.Is(err, context.Canceled) {
		return nil, err
	}
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, nil
	}

	return res[0].Messages, nil
}

func (mq *MQ) Ack(ctx context.Context, stream, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return mq.rdb.XAck(ctx, stream, group, ids...).Err()
}
