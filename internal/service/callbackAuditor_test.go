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

// activityForm 构造一条活动审核表单，status 为回调到达前的本地状态。
func activityForm(status string) *model.AuditorForm {
	return &model.AuditorForm{Id: 42, ActivityId: 7, Subject: model.SubjectActivity, Status: status}
}

// TestCallbackUpdateStatusPassesReason 回调带的拒绝原因必须随状态一起透传给 repo。
func TestCallbackUpdateStatusPassesReason(t *testing.T) {
	repo := &fakeAuditorRepo{byID: activityForm(model.StancePending)}
	svc := newCallbackAuditorWithAudForTest(repo, &stubAuditorSvc{status: model.StanceReject})

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
	repo := &fakeAuditorRepo{byID: activityForm(model.StancePending)}
	svc := newCallbackAuditorWithAudForTest(repo, &stubAuditorSvc{status: model.StanceReject})

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
	repo := &fakeAuditorRepo{byID: activityForm(model.StancePending)}
	svc := newCallbackAuditorWithAudForTest(repo, &stubAuditorSvc{status: model.StancePass})

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
	repo := &fakeAuditorRepo{byID: activityForm(model.StanceReject)}
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

// TestCallbackIgnoredWhenLocalMatchesPlatform 本地状态已与平台一致时，回调是旧的或重复的：
// 既不写库也不退回 pending，避免抖动，也避免驳回原因被短暂清空。
func TestCallbackIgnoredWhenLocalMatchesPlatform(t *testing.T) {
	for name, tc := range map[string]struct {
		local    string
		platform string
		callback string
		reason   string
	}{
		"本地已通过，重复的 pass 回调":   {model.StancePass, model.StancePass, "通过", ""},
		"本地已通过，迟到的 reject 回调": {model.StancePass, model.StancePass, "不通过", "资料不全"},
		"本地已驳回，迟到的 pass 回调":   {model.StanceReject, model.StanceReject, "通过", ""},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAuditorRepo{byID: activityForm(tc.local)}
			aud := &stubAuditorSvc{status: tc.platform}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, tc.callback, tc.reason); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if aud.queryCnt != 1 {
				t.Fatalf("expected platform queried once, got %d", aud.queryCnt)
			}
			if repo.updatedTo != "" {
				t.Fatalf("consistent local state must not be rewritten, got %q", repo.updatedTo)
			}
			if repo.parkCnt != 0 {
				t.Fatalf("consistent local state must not park form, got %d", repo.parkCnt)
			}
		})
	}
}

// TestCallbackBackfillsRejectReason 本地已驳回但没记原因（对账写入的 reject 不带原因），
// 随后带原因的驳回回调不能被当成重复回调丢掉，否则原因会永久缺失。
func TestCallbackBackfillsRejectReason(t *testing.T) {
	local := activityForm(model.StanceReject)
	local.Reason = ""
	repo := &fakeAuditorRepo{byID: local}
	aud := &stubAuditorSvc{status: model.StanceReject}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "不通过", "资料不全"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo != model.StanceReject {
		t.Fatalf("expected status reject, got %q", repo.updatedTo)
	}
	if repo.updatedReason != "资料不全" {
		t.Fatalf("expected reason backfilled, got %q", repo.updatedReason)
	}
}

// TestCallbackBackfillOnlyForRejectCallback 本地与平台都已驳回且本地缺原因时，
// 带消息的旧 pass 回调不能借"补原因"分支改写状态，否则会把已下架的活动重新上架。
func TestCallbackBackfillOnlyForRejectCallback(t *testing.T) {
	local := activityForm(model.StanceReject)
	local.Reason = ""
	repo := &fakeAuditorRepo{byID: local}
	aud := &stubAuditorSvc{status: model.StanceReject}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", "已通过"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedTo != "" {
		t.Fatalf("pass callback must not rewrite confirmed reject, got %q", repo.updatedTo)
	}
}

// TestCallbackTerminalIgnoredWhilePlatformPending 平台还没有结论时，终态回调可能只是
// 读延迟：应用它会把已发布活动短暂拉下来，退回 pending 同样会扰动，因此一律忽略。
func TestCallbackTerminalIgnoredWhilePlatformPending(t *testing.T) {
	for name, tc := range map[string]struct {
		local    string
		callback string
		reason   string
	}{
		"待审核收到 pass":   {model.StancePending, "通过", ""},
		"已发布收到 reject": {model.StancePass, "不通过", "资料不全"},
		"已驳回收到 pass":   {model.StanceReject, "通过", ""},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAuditorRepo{byID: activityForm(tc.local)}
			aud := &stubAuditorSvc{status: model.StancePending}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, tc.callback, tc.reason); err != nil {
				t.Fatalf("terminal callback must be ignored without error, got %v", err)
			}
			if repo.updatedTo != "" {
				t.Fatalf("terminal callback must not be applied while platform pending, got %q", repo.updatedTo)
			}
			if repo.parkCnt != 0 {
				t.Fatalf("terminal callback must not park form while platform pending, got %d", repo.parkCnt)
			}
		})
	}
}

// TestCallbackPendingToPassConfirmsPlatform 本地仍 pending（例如刚被退回待审）时，
// 平台重投的 pass 也必须先回查，确认一致才放行。
func TestCallbackPendingToPassConfirmsPlatform(t *testing.T) {
	var hits int
	aud, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
	})
	repo := &fakeAuditorRepo{byID: activityForm(model.StancePending)}
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
	if repo.parkCnt != 0 {
		t.Fatalf("confirmed pass must not park form, got %d", repo.parkCnt)
	}
}

// TestCallbackAlignsToPlatformWhenCallbackDiffers 平台已有结论而本地不同时，以平台为准对齐：
// 不采用回调结论（它已过期），也不退回 pending，避免活动在两边反复摆动。
func TestCallbackAlignsToPlatformWhenCallbackDiffers(t *testing.T) {
	for name, tc := range map[string]struct {
		local    string
		platform string
		callback string
	}{
		"本地待审，平台驳回，收到 pass":   {model.StancePending, model.StanceReject, "通过"},
		"本地待审，平台通过，收到 reject": {model.StancePending, model.StancePass, "不通过"},
		"本地已发布，平台驳回，收到 pass":  {model.StancePass, model.StanceReject, "通过"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAuditorRepo{byID: activityForm(tc.local)}
			aud := &stubAuditorSvc{status: tc.platform}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, tc.callback, "资料不全"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.updatedTo != tc.platform {
				t.Fatalf("expected align to platform %q, got %q", tc.platform, repo.updatedTo)
			}
			if repo.updatedReason != "" {
				t.Fatalf("platform status carries no reason, expected empty, got %q", repo.updatedReason)
			}
			if repo.parkCnt != 0 {
				t.Fatalf("aligned write must not park form, got %d", repo.parkCnt)
			}
		})
	}
}

// TestCallbackResetToPendingWritesWhenPlatformPending 平台把条目重置为未审核时，
// 确认一致后写 pending，由 AfterUpdate 把已出结论的活动退回审核中。
func TestCallbackResetToPendingWritesWhenPlatformPending(t *testing.T) {
	for name, local := range map[string]string{
		"本地已通过": model.StancePass,
		"本地已驳回": model.StanceReject,
		"本地待审核": model.StancePending,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAuditorRepo{byID: activityForm(local)}
			aud := &stubAuditorSvc{status: model.StancePending}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, "未审核", ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if aud.queryCnt != 1 {
				t.Fatalf("expected platform queried once, got %d", aud.queryCnt)
			}
			if repo.updatedTo != model.StancePending {
				t.Fatalf("confirmed reset should be written, got %q", repo.updatedTo)
			}
			if repo.parkCnt != 0 {
				t.Fatalf("confirmed reset must not park form, got %d", repo.parkCnt)
			}
		})
	}
}

// TestCallbackStaleResetIgnoredWhenPlatformConcluded 平台已给出结论时，延迟到达的重置回调
// 不得生效：此时既不能写 pending，也不必退回中间态。
func TestCallbackStaleResetIgnoredWhenPlatformConcluded(t *testing.T) {
	for name, platform := range map[string]string{
		"平台已通过": model.StancePass,
		"平台已驳回": model.StanceReject,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAuditorRepo{byID: activityForm(platform)}
			aud := &stubAuditorSvc{status: platform}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, "未审核", ""); err != nil {
				t.Fatalf("stale reset must return success, got %v", err)
			}
			if repo.updatedTo != "" {
				t.Fatalf("stale reset must not write status, got %q", repo.updatedTo)
			}
			if repo.parkCnt != 0 {
				t.Fatalf("stale reset must not park form, got %d", repo.parkCnt)
			}
		})
	}
}

// TestCallbackUnconfirmedParksAsPending 终态回调无法确认平台结论时不得应用：
// 未知状态、条目数异常、查询失败（含真实 HTTP 500）都只把表单退回 pending，
// 交由对账在平台给出结论后再决定（对账只扫 pending）。
func TestCallbackUnconfirmedParksAsPending(t *testing.T) {
	for name, tc := range map[string]struct {
		statusCode int
		body       string
	}{
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
			repo := &fakeAuditorRepo{byID: activityForm(model.StanceReject)}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hits != 1 {
				t.Fatalf("expected platform queried once, got %d", hits)
			}
			if repo.parkCnt != 1 || repo.parkFormId != 42 {
				t.Fatalf("unconfirmed platform status must park form as pending, got park=%d id=%d", repo.parkCnt, repo.parkFormId)
			}
			if repo.updatedTo != "" {
				t.Fatalf("unconfirmed platform status must not write status directly, got %q", repo.updatedTo)
			}
		})
	}
}

// TestCallbackResetUnconfirmedReturnsError 重置回调无法确认平台结论时必须返回 error 让平台重试：
// 退回 pending 会让对账的 UpdateIfPending 读到 pending == pending 早退，活动永远退不回审核中。
func TestCallbackResetUnconfirmedReturnsError(t *testing.T) {
	for name, tc := range map[string]struct {
		status string
		err    error
	}{
		"查询失败": {status: "", err: errors.New("platform timeout")},
		"状态未知": {status: "", err: nil},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAuditorRepo{byID: activityForm(model.StancePass)}
			aud := &stubAuditorSvc{status: tc.status, err: tc.err}
			svc := newCallbackAuditorWithAudForTest(repo, aud)

			if err := svc.UpdateStatus(context.Background(), 42, "未审核", ""); err == nil {
				t.Fatalf("unconfirmed reset callback must return error for platform retry")
			}
			if aud.queryCnt != 1 {
				t.Fatalf("expected platform queried once, got %d", aud.queryCnt)
			}
			if repo.updatedTo != "" {
				t.Fatalf("unconfirmed reset callback must not write status, got %q", repo.updatedTo)
			}
			if repo.parkCnt != 0 {
				t.Fatalf("unconfirmed reset callback must not park form, got %d", repo.parkCnt)
			}
		})
	}
}

// TestCallbackPostSkipsPlatformConfirm 帖子已不审核，其表单也不在对账范围内，
// 回调保持直接写库、不引入平台回查；否则平台条目缺失会让帖子回调永久停在 pending。
func TestCallbackPostSkipsPlatformConfirm(t *testing.T) {
	repo := &fakeAuditorRepo{byID: &model.AuditorForm{
		Id: 42, ActivityId: 7, Subject: model.SubjectPost, Status: model.StancePending,
	}}
	aud := &stubAuditorSvc{status: model.StancePass}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aud.queryCnt != 0 {
		t.Fatalf("post callback must not query platform, got %d", aud.queryCnt)
	}
	if repo.updatedTo != model.StancePass {
		t.Fatalf("post callback should be written directly, got %q", repo.updatedTo)
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
	if repo.updatedTo != "" || repo.parkCnt != 0 {
		t.Fatalf("find error must not write status, got update=%q park=%d", repo.updatedTo, repo.parkCnt)
	}
}

// TestCallbackFormMissingIsIdempotent 本地表单不存在时保持幂等成功：不回查平台、不写库。
func TestCallbackFormMissingIsIdempotent(t *testing.T) {
	repo := &fakeAuditorRepo{}
	aud := &stubAuditorSvc{status: model.StancePass}
	svc := newCallbackAuditorWithAudForTest(repo, aud)

	if err := svc.UpdateStatus(context.Background(), 42, "通过", ""); err != nil {
		t.Fatalf("missing form must be idempotent success, got %v", err)
	}
	if aud.queryCnt != 0 {
		t.Fatalf("missing form should skip platform query, got %d", aud.queryCnt)
	}
	if repo.updatedTo != "" || repo.parkCnt != 0 {
		t.Fatalf("missing form must not write status, got update=%q park=%d", repo.updatedTo, repo.parkCnt)
	}
}
