package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

func newCallbackAuditorForTest(repo *fakeAuditorRepo) CallbackAuditorService {
	return NewCallbackAuditor(repo, &logger.LoggerSet{Auditor: zap.NewNop()})
}

// TestCallbackUpdateStatusPassesReason 回调带的拒绝原因必须随状态一起透传给 repo。
func TestCallbackUpdateStatusPassesReason(t *testing.T) {
	repo := &fakeAuditorRepo{}
	svc := newCallbackAuditorForTest(repo)

	if err := svc.UpdateStatus(context.Background(), 42, "不通过", "资料不全"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo != "reject" {
		t.Fatalf("expected status reject, got %q", repo.updatedTo)
	}
	if repo.updatedReason != "资料不全" {
		t.Fatalf("expected reason to be passed through, got %q", repo.updatedReason)
	}
}

// TestCallbackUpdateStatusInvalidStatus 非法状态返回 ErrAuditorStatusInvalid，且不写库。
func TestCallbackUpdateStatusInvalidStatus(t *testing.T) {
	repo := &fakeAuditorRepo{}
	svc := newCallbackAuditorForTest(repo)

	err := svc.UpdateStatus(context.Background(), 42, "unknown", "资料不全")
	if !errors.Is(err, errs.ErrAuditorStatusInvalid) {
		t.Fatalf("expected ErrAuditorStatusInvalid, got %v", err)
	}
	if repo.updatedTo != "" {
		t.Fatalf("invalid status must not update repo, got %q", repo.updatedTo)
	}
}

// TestCallbackUpdateStatusTruncatesLongReason 超长原因按 rune 截断到列宽 512，
// 避免 MySQL 1406 把状态也一起写失败。
func TestCallbackUpdateStatusTruncatesLongReason(t *testing.T) {
	repo := &fakeAuditorRepo{}
	svc := newCallbackAuditorForTest(repo)

	long := strings.Repeat("拒", 600)
	if err := svc.UpdateStatus(context.Background(), 42, "不通过", long); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo != "reject" {
		t.Fatalf("status must still be written on truncation, got %q", repo.updatedTo)
	}
	if got := len([]rune(repo.updatedReason)); got != model.AuditorReasonMaxLen {
		t.Fatalf("expected reason truncated to %d runes, got %d", model.AuditorReasonMaxLen, got)
	}
}

// TestCallbackUpdateStatusNoTruncateForNonReject 非 reject 的原因不会入库，不截断也不告警，
// 避免日志出现误导性的 truncated 记录。
func TestCallbackUpdateStatusNoTruncateForNonReject(t *testing.T) {
	repo := &fakeAuditorRepo{}
	svc := newCallbackAuditorForTest(repo)

	long := strings.Repeat("拒", 600)
	if err := svc.UpdateStatus(context.Background(), 42, "通过", long); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len([]rune(repo.updatedReason)); got != 600 {
		t.Fatalf("non-reject reason must not be truncated, got %d runes", got)
	}
}
