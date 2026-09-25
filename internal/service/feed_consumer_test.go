package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type fakeFeedMQ struct {
	acked     []string
	published []string
	claimMsgs []redis.XMessage
	pending   []redis.XPendingExt
	starts    []string
}

func (m *fakeFeedMQ) Publish(_ context.Context, stream string, _ interface{}) error {
	m.published = append(m.published, stream)
	return nil
}

func (m *fakeFeedMQ) EnsureConsumerGroup(context.Context, string, string) error { return nil }

func (m *fakeFeedMQ) ConsumeGroup(context.Context, string, string, string, int64, time.Duration) ([]redis.XMessage, error) {
	return nil, nil
}

func (m *fakeFeedMQ) Ack(_ context.Context, _, _ string, ids ...string) error {
	m.acked = append(m.acked, ids...)
	return nil
}

func (m *fakeFeedMQ) AutoClaim(_ context.Context, _, _, _ string, _ time.Duration, start string) ([]redis.XMessage, string, error) {
	m.starts = append(m.starts, start)
	return m.claimMsgs, "0-0", nil
}

func (m *fakeFeedMQ) ListPendingExt(context.Context, string, string, time.Duration, string, string, int64) ([]redis.XPendingExt, error) {
	return m.pending, nil
}

func newFeedServiceForTest(t *testing.T) (*FeedService, *fakeFeedMQ, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy:         schema.NamingStrategy{SingularTable: true},
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	fd := dao.NewFeedDao(gdb, &logger.LoggerSet{Feed: zap.NewNop()})
	fmq := &fakeFeedMQ{}
	return &FeedService{fd: fd, mq: fmq, l: zap.NewNop()}, fmq, mock
}

func feedMsg(data string) redis.XMessage {
	return redis.XMessage{ID: "1-0", Values: map[string]interface{}{"data": data}}
}

const feedTestPayload = `{"target_id":1,"object":"post","student_id":"S1","receiver":"S2","action":"like"}`

// 入库失败不得 ACK：消息须留在 PEL 等重投。
func TestFeedProcess_PersistFailureDoesNotAck(t *testing.T) {
	fs, fmq, mock := newFeedServiceForTest(t)
	mock.ExpectExec("INSERT INTO `feed`").WillReturnError(errors.New("db down"))

	fs.processFeedMessage(context.Background(), feedMsg(feedTestPayload))

	if len(fmq.acked) != 0 {
		t.Fatalf("persist failure must not ack, got acked=%v", fmq.acked)
	}
}

// 入库成功应 ACK 一次。
func TestFeedProcess_SuccessAcks(t *testing.T) {
	fs, fmq, mock := newFeedServiceForTest(t)
	mock.ExpectExec("INSERT INTO `feed`").WillReturnResult(sqlmock.NewResult(1, 1))

	fs.processFeedMessage(context.Background(), feedMsg(feedTestPayload))

	if len(fmq.acked) != 1 {
		t.Fatalf("success should ack once, got %v", fmq.acked)
	}
}

// 解析失败属永久性错误：ACK 丢弃，避免永远占用 PEL。
func TestFeedProcess_MalformedAcksAndDrops(t *testing.T) {
	fs, fmq, _ := newFeedServiceForTest(t)

	fs.processFeedMessage(context.Background(), feedMsg("not-json"))

	if len(fmq.acked) != 1 {
		t.Fatalf("malformed poison must be acked and dropped, got %v", fmq.acked)
	}
}

// 重投且超过重试上限：先写 DLQ 再 ACK。
func TestFeedRecover_OverRetryLimitMovesToDLQ(t *testing.T) {
	fs, fmq, _ := newFeedServiceForTest(t)
	msg := feedMsg(feedTestPayload)
	fmq.claimMsgs = []redis.XMessage{msg}
	fmq.pending = []redis.XPendingExt{{ID: msg.ID, RetryCount: feedMaxRetry}}

	fs.recoverFeedPending(context.Background(), "consumer-1")

	if len(fmq.published) != 1 || fmq.published[0] != feedDLQKey {
		t.Fatalf("expected publish to %s, got %v", feedDLQKey, fmq.published)
	}
	if len(fmq.acked) != 1 || fmq.acked[0] != msg.ID {
		t.Fatalf("expected ack after DLQ publish, got %v", fmq.acked)
	}
}

// XAUTOCLAIM 起始游标必须合法："0-0"，不能是空串（Redis 会报 Invalid stream ID）。
func TestFeedRecover_UsesValidStartCursor(t *testing.T) {
	fs, fmq, _ := newFeedServiceForTest(t)

	fs.recoverFeedPending(context.Background(), "consumer-1")

	if len(fmq.starts) == 0 || fmq.starts[0] != "0-0" {
		t.Fatalf("first XAUTOCLAIM start must be \"0-0\", got %v", fmq.starts)
	}
}

// 未超限的重投正常再处理：成功入库后 ACK，不进 DLQ。
func TestFeedRecover_UnderLimitReprocessesAndAcks(t *testing.T) {
	fs, fmq, mock := newFeedServiceForTest(t)
	msg := feedMsg(feedTestPayload)
	fmq.claimMsgs = []redis.XMessage{msg}
	fmq.pending = []redis.XPendingExt{{ID: msg.ID, RetryCount: 1}}
	mock.ExpectExec("INSERT INTO `feed`").WillReturnResult(sqlmock.NewResult(1, 1))

	fs.recoverFeedPending(context.Background(), "consumer-1")

	if len(fmq.published) != 0 {
		t.Fatalf("under retry limit must not go to DLQ, got %v", fmq.published)
	}
	if len(fmq.acked) != 1 || fmq.acked[0] != msg.ID {
		t.Fatalf("reprocessed message should be acked, got %v", fmq.acked)
	}
}
