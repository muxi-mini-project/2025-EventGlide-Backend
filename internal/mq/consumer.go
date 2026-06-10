package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	DLQKey = "interaction_dlq"
	ConsumeCount  = 100
	ConsumeBlock  = 5 * time.Second
	MaxRetryCount = 3
	RecoverIdle = 2 * time.Minute
)

// InteractionConsumer MQ 消费者，处理互动事件
type InteractionConsumer struct {
	mq       MQHdl
	dao      *dao.InteractionDao
	l *zap.Logger
	consumer string
	group    string
}

// NewInteractionConsumer 创建互动事件消费者
func NewInteractionConsumer(mq MQHdl, dao *dao.InteractionDao, l *zap.Logger) *InteractionConsumer {
	return &InteractionConsumer{
		mq:       mq,
		dao:      dao,
		l:        l,
		consumer: "interaction-consumer",
		group:    "interaction-group",
	}
}

// Start 启动消费循环
func (c *InteractionConsumer) Start(ctx context.Context) error {
	if err := c.mq.EnsureConsumerGroup(ctx, StreamKey, c.group); err != nil {
		return err
	}

	go c.recoverLoop(ctx)
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

// recoverLoop 定时扫描 PEL 中的_pending 消息
func (c *InteractionConsumer) recoverLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.recoverPending(ctx)
		}
	}
}

// recoverPending 使用 XAUTOCLAIM 捡回空闲超_RecoverIdle_秒的消息
func (c *InteractionConsumer) recoverPending(ctx context.Context) {
	start := ""
	for {
		msgs, nextStart, err := c.mq.AutoClaim(ctx, StreamKey, c.group, c.consumer, RecoverIdle, start)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return
			}
			c.l.Error("AutoClaim failed", zap.Error(err))
			return
		}
		if len(msgs) == 0 {
			return
		}
		c.processRecoveredMessages(ctx, msgs)
		start = nextStart
		if nextStart == "" || nextStart == "0-0" {
			return
		}
	}
}

// processRecoveredMessages 处理从 PEL 捡回的消息，超限则入 DLQ
func (c *InteractionConsumer) processRecoveredMessages(ctx context.Context, msgs []redis.XMessage) {
	for _, msg := range msgs {
		// 查 delivery count
		pending, err := c.mq.ListPendingExt(ctx, StreamKey, c.group, 0, msg.ID, msg.ID, 1)
		if err != nil {
			c.l.Error("ListPendingExt failed", zap.Error(err), zap.String("msg_id", msg.ID))
			continue
		}
		var event InteractionEvent
		data, ok := msg.Values["data"].(string)
		if !ok {
			c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
			continue
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			c.l.Error("Unmarshal failed", zap.Error(err), zap.String("data", data))
			c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
			continue
		}

		// delivery count 超过阈值，进入 DLQ
		if len(pending) > 0 && pending[0].RetryCount >= MaxRetryCount {
			c.l.Warn("Message delivery count exceeded, moving to DLQ",
				zap.String("msg_id", msg.ID),
				zap.Int64("retry_count", pending[0].RetryCount),
				zap.Any("event", event))
			c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
			c.mq.Publish(ctx, DLQKey, event)
			continue
		}

		// 未超限，正常处理
		if err := c.handleEvent(ctx, &event); err != nil {
			if strings.Contains(err.Error(), "unknown interaction type") ||
				strings.Contains(err.Error(), "unknown") ||
				strings.Contains(err.Error(), "invalid") {
				c.l.Warn("Non-retryable error, ack and skip",
					zap.String("msg_id", msg.ID),
					zap.Error(err),
					zap.Any("event", event))
				c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
				continue
			}
			// 可重试错误：留在 PEL，等待下次 recoverLoop
			c.l.Warn("Retryable error, will retry via PEL",
				zap.String("msg_id", msg.ID),
				zap.Error(err),
				zap.Any("event", event))
			continue
		}
		c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
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
			//不可重试错误：直接 ACK丢弃
			if strings.Contains(err.Error(), "unknown interaction type") ||
				strings.Contains(err.Error(), "unknown") ||
				strings.Contains(err.Error(), "invalid") {
				c.l.Warn("Non-retryable error, ack and skip",
					zap.String("msg_id", msg.ID),
					zap.Error(err),
					zap.Any("event", event))
				c.mq.Ack(ctx, StreamKey, c.group, msg.ID)
				continue
			}
			// 可重试错误：留在 PEL，由 recoverLoop 的 XAUTOCLAIM 重新消费
			c.l.Warn("Retryable error, will retry via PEL",
				zap.String("msg_id", msg.ID),
				zap.Error(err),
				zap.Any("event", event))
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
	switch event.Action {
	case "add":
		return c.dao.InsertLike(ctx, event.Subject, event.SubjectID, event.UserID)
	case "remove":
		return c.dao.DeleteLike(ctx, event.Subject, event.SubjectID, event.UserID)
	default:
		return fmt.Errorf("unknown like action: %s", event.Action)
	}
}

// handleCollect 处理收藏事件
func (c *InteractionConsumer) handleCollect(ctx context.Context, event *InteractionEvent) error {
	switch event.Action {
	case "add":
		return c.dao.InsertCollect(ctx, event.Subject, event.SubjectID, event.UserID)
	case "remove":
		return c.dao.DeleteCollect(ctx, event.Subject, event.SubjectID, event.UserID)
	default:
		return fmt.Errorf("unknown collect action: %s", event.Action)
	}
}
