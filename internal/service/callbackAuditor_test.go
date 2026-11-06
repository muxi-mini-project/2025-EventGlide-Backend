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

// TestCallbackPassSkipsConfirmWhenAlreadyPass 本地已通过时幂等，不回查平台。
func TestCallbackPassSkipsConfirmWhenAlreadyPass(t *testing.T) {
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: model.StancePass,
	}}
	aud := &stubAuditorSvc{status: model.StancePass}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aud.queryCnt != 0 {
		t.Fatalf("already-pass form should not query platform, got %d queries", aud.queryCnt)
	}
	if repo.updatedTo != model.StancePass {
		t.Fatalf("expected pass written, got %q", repo.updatedTo)
	}
}

// TestCallbackPendingToPassConfirmsPlatform 本地仍 pending（例如刚被退回待审）时，
// 平台重投的 pass 也必须先回查，防止绕过确认直接放行。
func TestCallbackPendingToPassConfirmsPlatform(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
	})
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: "pending",
	}}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 1 {
		t.Fatalf("pending form must query platform once, got %d", hits)
	}
	if repo.updatedTo != model.StancePass {
		t.Fatalf("confirmed pass should be written, got %q", repo.updatedTo)
	}
}

// TestCallbackPendingToPassIgnoredWhenPlatformRejects 本地 pending、平台仍是 reject 时，
// 这条 pass 与平台当前结论冲突，忽略且不改本地状态。
func TestCallbackPendingToPassIgnoredWhenPlatformRejects(t *testing.T) {
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: "pending",
	}}
	aud := &stubAuditorSvc{status: model.StanceReject}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("ignored callback must return success, got %v", err)
	}
	if aud.queryCnt != 1 {
		t.Fatalf("expected platform queried once, got %d", aud.queryCnt)
	}
	if repo.updatedTo != "" || repo.revertCnt != 0 {
		t.Fatalf("conflicting pass must not change status, got update=%q revert=%d", repo.updatedTo, repo.revertCnt)
	}
}

// TestCallbackPendingPassStaysPending 平台仍 pending 时无法确认结论，退回操作对 pending 行
// 是条件更新的空操作（真实 DAO 命中 0 行），因此不会有任何状态写入。
func TestCallbackPendingPassStaysPending(t *testing.T) {
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: "pending",
	}}
	aud := &stubAuditorSvc{status: "pending"}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo != "" {
		t.Fatalf("unconfirmed pass must not be written, got %q", repo.updatedTo)
	}
	if repo.revertCnt != 1 {
		t.Fatalf("expected revert attempted once (no-op on pending), got %d", repo.revertCnt)
	}
}

// TestCallbackRejectToPassRevertsToPending 平台结论无法确认时不得放行，也不能丢弃回调：
// pending、未知状态、条目数异常、查询失败（含真实 HTTP 500）都把本地表单退回 pending，
// 交由对账在平台给出结论后再决定（对账只扫 pending）。
func TestCallbackRejectToPassRevertsToPending(t *testing.T) {
	for name, tc := range map[string]struct {
		statusCode int
		body       string
	}{
		"pending":     {http.StatusOK, `{"msg":"","code":200,"data":{"items":[{"status":0}]}}`},
		"unknown":     {http.StatusOK, `{"msg":"","code":200,"data":{"items":[{"status":99}]}}`},
		"empty items": {http.StatusOK, `{"msg":"","code":200,"data":{"items":[]}}`},
		"http 500":    {http.StatusInternalServerError, `boom`},
	} {
		t.Run(name, func(t *testing.T) {
			var hits int
			aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
				hits++
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
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
			if repo.revertCnt != 1 || repo.revertFormId != 42 {
				t.Fatalf("unconfirmed platform status must revert form to pending, got revert=%d id=%d", repo.revertCnt, repo.revertFormId)
			}
			if repo.updatedTo != "" {
				t.Fatalf("unconfirmed platform status must not write status directly, got %q", repo.updatedTo)
			}
		})
	}
}

// TestCallbackFindByIDErrorReturnsError 读本地表单失败时无法判断是否需要回查，
// 必须返回 error 让平台重试（最终由对账兜底），不能 fail-open 直接放行。
func TestCallbackFindByIDErrorReturnsError(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
	})
	repo := &fakeAuditorRepo{findErr: errors.New("db down")}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err == nil {
		t.Fatalf("find error must be returned for platform retry")
	}
	if hits != 0 {
		t.Fatalf("find error should skip platform query, got %d hits", hits)
	}
	if repo.updatedTo != "" {
		t.Fatalf("find error must not write pass, got %q", repo.updatedTo)
	}
}
