package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/raiki02/EG/internal/dao"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// InteractionEvent 互动事件
type InteractionEvent struct {
	Type      string `json:"type"`    // like, collect
	Action    string `json:"action"`  // add, remove
	Subject   string `json:"subject"` // activity, post, comment
	SubjectID int64  `json:"subject_id"`
	UserID    int64  `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
}

const (
	StreamKey     = "interaction_stream"
	ConsumeCount  = 100
	ConsumeBlock  = 5 * time.Second
	MaxRetryCount = 3
)

// InteractionConsumer MQ 消费者，处理互动事件
type InteractionConsumer struct {
	mq          MQHdl
	dao         *dao.InteractionDao
	l           *zap.Logger
	consumer    string
	group       string
	retryMu     sync.RWMutex
	retryCounts map[string]int //消息ID -> 重试次数
}

// NewInteractionConsumer 创建互动事件消费者
func NewInteractionConsumer(mq MQHdl, dao *dao.InteractionDao, l *zap.Logger) *InteractionConsumer {
	return &InteractionConsumer{
		mq:          mq,
		dao:         dao,
		l:           l,
		consumer:    "interaction-consumer",
		group:       "interaction-group",
		retryCounts: make(map[string]int),
	}
}

// Start 启动消费循环
func (c *InteractionConsumer) Start(ctx context.Context) error {
	if err := c.mq.EnsureConsumerGroup(ctx, StreamKey, c.group); err != nil {
		return err
	}

	go c.consumeLoop(ctx)
	return nil
}

// consumeLoop 消费循环
func (c *InteractionConsumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msgs, err := c.mq.ConsumeGroup(ctx, StreamKey, c.group, c.consumer, ConsumeCount, ConsumeBlock)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				c.l.Error("Consume failed", zap.Error(err))
				time.Sleep(time.Second)
				continue
			}
			if len(msgs) == 0 {
				continue
			}
			c.processMessages(ctx, msgs)
		}
	}
}

// processMessages 处理消息列表
func (c *InteractionConsumer) processMessages(ctx context.Context, msgs []redis.XMessage) {
	for _, msg := range msgs {
		var event InteractionEvent
		data, ok := msg.Values["data"].(string)
		if !ok {
			c.l.Warn("Invalid message format", zap.Any("msg", msg))
			c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
			continue
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			c.l.Error("Unmarshal failed", zap.Error(err), zap.String("data", data))
			c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
			continue
		}

		if err := c.handleEvent(ctx, &event); err != nil {
			// 未知类型错误，不重试，直接 ACK
			if strings.Contains(err.Error(), "unknown interaction type") {
				c.l.Error("Unknown interaction type, skipping", zap.Any("event", event))
				c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
				continue
			}
			c.l.Error("Handle event failed", zap.Error(err), zap.Any("event", event))
			// 追踪重试次数，超过阈值则 ACK 并告警
			c.retryMu.Lock()
			c.retryCounts[msg.ID]++
			if c.retryCounts[msg.ID] >= MaxRetryCount {
				c.l.Error("Message retry exceeded, ack and skip",
					zap.String("msg_id", msg.ID),
					zap.Any("event", event))
				delete(c.retryCounts, msg.ID)
				c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
			}
			c.retryMu.Unlock()
			continue
		}

		c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
	}
}

// handleEvent 根据事件类型分发处理
func (c *InteractionConsumer) handleEvent(ctx context.Context, event *InteractionEvent) error {
	switch event.Type {
	case "like":
		return c.handleLike(ctx, event)
	case "collect":
		return c.handleCollect(ctx, event)
	default:
		c.l.Warn("Unknown type", zap.String("type", event.Type))
		return fmt.Errorf("unknown interaction type: %s", event.Type)
	}
}

// handleLike 处理点赞事件
func (c *InteractionConsumer) handleLike(ctx context.Context, event *InteractionEvent) error {
	if event.Action == "add" {
		return c.dao.InsertLike(ctx, event.Subject, event.SubjectID, event.UserID)
	} else if event.Action == "remove" {
		return c.dao.DeleteLike(ctx, event.Subject, event.SubjectID, event.UserID)
	}
	return nil
}

// handleCollect 处理收藏事件
func (c *InteractionConsumer) handleCollect(ctx context.Context, event *InteractionEvent) error {
	if event.Action == "add" {
		return c.dao.InsertCollect(ctx, event.Subject, event.SubjectID, event.UserID)
	} else if event.Action == "remove" {
		return c.dao.DeleteCollect(ctx, event.Subject, event.SubjectID, event.UserID)
	}
	return nil
}
