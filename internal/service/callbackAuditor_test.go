package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

// stubAuditorSvc 只记录 PlatformStatus 调用，其余方法不属于回调路径。
type stubAuditorSvc struct {
	AuditorService
	status   string
	err      error
	queryCnt int
}

func (s *stubAuditorSvc) PlatformStatus(context.Context, int64) (string, error) {
	s.queryCnt++
	return s.status, s.err
}

func newCallbackAuditorForTest(repo *fakeAuditorRepo) CallbackAuditorService {
	return newCallbackAuditorWithAudForTest(repo, &stubAuditorSvc{})
}

func newCallbackAuditorWithAudForTest(repo *fakeAuditorRepo, aud AuditorService) CallbackAuditorService {
	return NewCallbackAuditor(repo, aud, &logger.LoggerSet{Auditor: zap.NewNop()})
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

// TestCallbackRejectToPassConfirmsPlatform 本地已驳回、平台当前确为 pass 时才放行并写库。
func TestCallbackRejectToPassConfirmsPlatform(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
	})
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: model.StanceReject,
	}}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected platform queried once, got %d", hits)
	}
	if repo.updatedTo != model.StancePass {
		t.Fatalf("confirmed pass should be written, got %q", repo.updatedTo)
	}
}

// TestCallbackRejectToPassIgnoredWhenPlatformRejects 平台当前仍是 reject 时，
// 说明这条 pass 回调是延迟到达的旧结论，必须忽略（不写库）且返回成功避免平台重试。
func TestCallbackRejectToPassIgnoredWhenPlatformRejects(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":2}]}}`))
	})
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: model.StanceReject,
	}}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("ignored callback must return success, got %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected platform queried once, got %d", hits)
	}
	if repo.updatedTo != "" {
		t.Fatalf("stale pass callback must not update repo, got %q", repo.updatedTo)
	}
}

// TestCallbackPassSkipsConfirmWhenNotRejected 本地是 pending 或已 pass 时无需回查平台，照旧写库。
func TestCallbackPassSkipsConfirmWhenNotRejected(t *testing.T) {
	for _, local := range []string{"pending", model.StancePass} {
		t.Run(local, func(t *testing.T) {
			var hits int
			aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
				hits++
				_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
			})
			repo := &fakeAuditorRepo{byID: &model.AuditorForm{
				Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: local,
			}}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hits != 0 {
				t.Fatalf("local %q should not query platform, got %d hits", local, hits)
			}
			if repo.updatedTo != model.StancePass {
				t.Fatalf("expected pass written for local %q, got %q", local, repo.updatedTo)
			}
		})
	}
}

// TestCallbackPlatformStatusErrorFailsOpen 平台回查出错时保持放行，不新增失败面。
func TestCallbackPlatformStatusErrorFailsOpen(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"code":500,"msg":"boom"}`))
	})
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: model.StanceReject,
	}}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("confirm error must not fail the callback, got %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected platform queried once, got %d", hits)
	}
	if repo.updatedTo != model.StancePass {
		t.Fatalf("confirm error should fail open and write pass, got %q", repo.updatedTo)
	}
}

// TestCallbackRejectToPassAcceptsWhenPlatformPending 平台当前仍在审核中时不能丢弃这条 pass：
// 本地已停在 reject，而对账只扫 pending，丢弃后活动将永久无法恢复。
func TestCallbackRejectToPassAcceptsWhenPlatformPending(t *testing.T) {
	for name, body := range map[string]string{
		"pending": `{"msg":"","code":200,"data":{"items":[{"status":0}]}}`,
		"unknown": `{"msg":"","code":200,"data":{"items":[{"status":99}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var hits int
			aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
				hits++
				_, _ = w.Write([]byte(body))
			})
			repo := &fakeAuditorRepo{byID: &model.AuditorForm{
				Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: model.StanceReject,
			}}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hits != 1 {
				t.Fatalf("expected platform queried once, got %d", hits)
			}
			if repo.updatedTo != model.StancePass {
				t.Fatalf("platform %s should not block pass callback, got %q", name, repo.updatedTo)
			}
		})
	}
}

// TestCallbackFindByIDErrorFailsOpen 读本地表单失败时无法判断是否需要回查，
// 保持放行以免平台抖动导致回调落库失败。
func TestCallbackFindByIDErrorFailsOpen(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
	})
	repo := &fakeAuditorRepo{findErr: errors.New("db down")}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("find error must not fail the callback, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("find error should skip platform query, got %d hits", hits)
	}
	if repo.updatedTo != model.StancePass {
		t.Fatalf("find error should fail open and write pass, got %q", repo.updatedTo)
	}
}
