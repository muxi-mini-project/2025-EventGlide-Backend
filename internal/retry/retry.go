package retry

import (
	"context"
	"sync"
	"time"
)

// MQMessage 消息队列消息结构
type MQMessage struct {
	Stream     string
	Data       interface{}
	RetryCount int // 已重试次数
}

// RetryQueue 重试队列
type RetryQueue struct {
	queue         []MQMessage
	mu            sync.Mutex
	maxRetries    int
	retryInterval time.Duration
}

// NewRetryQueue 创建重试队列实例
func NewRetryQueue(maxRetries int, retryInterval time.Duration) *RetryQueue {
	return &RetryQueue{
		queue:         make([]MQMessage, 0),
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
	}
}

// Add 添加重试消息（初始重试次数为0）
func (q *RetryQueue) Add(stream string, data interface{}) {
	q.AddWithRetry(stream, data, 0)
}

// AddWithRetry 添加重试消息并指定初始重试次数
func (q *RetryQueue) AddWithRetry(stream string, data interface{}, retryCount int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, MQMessage{Stream: stream, Data: data, RetryCount: retryCount})
}

// StartRetryLoop 启动重试循环
func (q *RetryQueue) StartRetryLoop(ctx context.Context, publishFn func(ctx context.Context, stream string, data interface{}) error) {
	ticker := time.NewTicker(q.retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.mu.Lock()
			if len(q.queue) == 0 {
				q.mu.Unlock()
				continue
			}
			messages := q.queue
			q.queue = make([]MQMessage, 0)
			q.mu.Unlock()

			for _, msg := range messages {
				if err := publishFn(ctx, msg.Stream, msg.Data); err != nil {
					if msg.RetryCount >= q.maxRetries {
						// 超过最大重试次数，记录日志（死信）
						continue // 跳过，不再重试
					}
					// 重试失败，增加计数后重新入队等待下次重试
					msg.RetryCount++
					q.mu.Lock()
					q.queue = append(q.queue, msg)
					q.mu.Unlock()
				}
			}
		}
	}
}
