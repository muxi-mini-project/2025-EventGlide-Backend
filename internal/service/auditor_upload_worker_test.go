package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

type fakePendingActivities struct {
	acts []model.Activity
}

func (f *fakePendingActivities) FindPendingAuditorActivities(context.Context) ([]model.Activity, error) {
	return f.acts, nil
}

// fakeAuditorSvc 以单个内存 form 模拟审核服务，用于验证 worker 的幂等与重试序列。
type fakeAuditorSvc struct {
	form         *model.AuditorForm
	nextId       int64
	uploadErr    error
	uploadCnt    int
	reconcileCnt int
	lastSub      string
	lastFormUrl  string
	reasons      map[int64]string // GetReasons 的返回值
	reasonIds    []int64          // GetReasons 收到的 activity id
}

var _ AuditorService = (*fakeAuditorSvc)(nil)

func (f *fakeAuditorSvc) GetOrCreatePendingForm(_ context.Context, id int64, formUrl, sub string) (*model.AuditorForm, error) {
	f.lastSub = sub
	f.lastFormUrl = formUrl
	if f.form == nil {
		f.nextId++
		f.form = &model.AuditorForm{Id: f.nextId, ActivityId: id, Subject: sub, FormUrl: formUrl}
		return f.form, nil
	}
	if f.form.PushedAt != nil {
		return nil, ErrFormAlreadyPushed
	}
	return f.form, nil
}

func (f *fakeAuditorSvc) ClaimForUpload(context.Context, int64) (time.Time, bool, error) {
	if f.form == nil || f.form.PushedAt != nil || f.form.ClaimedAt != nil {
		return time.Time{}, false, nil
	}
	lease := time.Now().Add(time.Minute)
	f.form.ClaimedAt = &lease
	return lease, true, nil
}

func (f *fakeAuditorSvc) ReleaseClaim(_ context.Context, _ int64, leaseUntil time.Time) error {
	if f.form != nil && f.form.ClaimedAt != nil && f.form.ClaimedAt.Equal(leaseUntil) {
		f.form.ClaimedAt = nil
	}
	return nil
}

func (f *fakeAuditorSvc) UploadForm(context.Context, *req.AuditWrapper, int64) error {
	f.uploadCnt++
	return f.uploadErr
}

func (f *fakeAuditorSvc) MarkPushed(_ context.Context, _ int64, pushedAt time.Time) error {
	if f.form == nil {
		return errors.New("no form")
	}
	now := pushedAt
	f.form.PushedAt = &now
	f.form.ClaimedAt = nil
	return nil
}

func (f *fakeAuditorSvc) CreateAuditorForm(context.Context, int64, string, string) (*model.AuditorForm, error) {
	return nil, errors.New("not used")
}

func (f *fakeAuditorSvc) ReconcilePendingForms(context.Context) {
	f.reconcileCnt++
}

func (f *fakeAuditorSvc) GetReasons(_ context.Context, ids []int64) (map[int64]string, error) {
	f.reasonIds = ids
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		if reason, ok := f.reasons[id]; ok {
			out[id] = reason
		}
	}
	return out, nil
}

func (f *fakeAuditorSvc) PlatformStatus(context.Context, int64) (string, error) {
	return "", errors.New("not used")
}

func newWorkerForTest(acts *fakePendingActivities, svc AuditorService) *AuditorUploadWorker {
	return &AuditorUploadWorker{
		activityRepo:   acts,
		auditorService: svc,
		logger:         &logger.LoggerSet{Auditor: zap.NewNop()},
	}
}

// TestActivityUpload_IdempotentAcrossTicks 首个 tick 成功送审后，后续 tick 因 form 已推送而跳过，
// 不重复远端上传——这是"回调到达前每 5s 重复送审"的回归点。
func TestActivityUpload_IdempotentAcrossTicks(t *testing.T) {
	acts := &fakePendingActivities{acts: []model.Activity{{Id: 1001, StudentID: "S1", Title: "t", ActiveForm: "https://form"}}}
	svc := &fakeAuditorSvc{}
	w := newWorkerForTest(acts, svc)

	w.processPendingAuditorActivities(context.Background())
	if svc.uploadCnt != 1 {
		t.Fatalf("first tick should upload once, got %d", svc.uploadCnt)
	}
	if svc.lastSub != SubjectActivity || svc.lastFormUrl != "https://form" {
		t.Fatalf("activity form must use subject=%q formUrl=%q, got subject=%q formUrl=%q", SubjectActivity, "https://form", svc.lastSub, svc.lastFormUrl)
	}

	w.processPendingAuditorActivities(context.Background())
	if svc.uploadCnt != 1 {
		t.Fatalf("already-pushed form must not be re-uploaded, got %d uploads", svc.uploadCnt)
	}
}

// TestActivityUpload_RetriesAfterFailure 上传失败应释放占用，使下个 tick 能立即重试成功。
func TestActivityUpload_RetriesAfterFailure(t *testing.T) {
	acts := &fakePendingActivities{acts: []model.Activity{{Id: 1001, StudentID: "S1", Title: "t", ActiveForm: "https://form"}}}
	svc := &fakeAuditorSvc{uploadErr: errors.New("network")}
	w := newWorkerForTest(acts, svc)

	w.processPendingAuditorActivities(context.Background())
	if svc.uploadCnt != 1 {
		t.Fatalf("first tick should attempt upload once, got %d", svc.uploadCnt)
	}
	if svc.form.PushedAt != nil {
		t.Fatalf("form must not be marked pushed after a failed upload")
	}
	if svc.form.ClaimedAt != nil {
		t.Fatalf("claim should be released after a failed upload so next tick can retry")
	}

	svc.uploadErr = nil
	w.processPendingAuditorActivities(context.Background())
	if svc.uploadCnt != 2 {
		t.Fatalf("next tick should retry the upload, got %d", svc.uploadCnt)
	}
	if svc.form.PushedAt == nil {
		t.Fatalf("form should be marked pushed after a successful retry")
	}
}

// TestWorkerReconcileInvokesService 对账 tick 应把控制权交给 AuditorService.ReconcilePendingForms。
func TestWorkerReconcileInvokesService(t *testing.T) {
	svc := &fakeAuditorSvc{}
	w := newWorkerForTest(&fakePendingActivities{}, svc)

	w.reconcile()
	if svc.reconcileCnt != 1 {
		t.Fatalf("expected reconcile invoked once, got %d", svc.reconcileCnt)
	}
}

// tickCtxKey 标记"整轮 tick 的 ctx"。记录处理必须使用独立 ctx，不能携带该标记。
type tickCtxKey struct{}

// ctxCheckingSvc 在每一步校验：入参 ctx 若携带 tickCtxKey（即复用了整轮 tick 的 ctx）则失败，
// 用于验证单条记录使用独立 ctx（历史事故：一条挂死的上传耗尽整轮预算，后续记录全报 ctx deadline exceeded）。
type ctxCheckingSvc struct {
	form       *model.AuditorForm
	uploadCnt  int
	sawTickCtx bool
}

var _ AuditorService = (*ctxCheckingSvc)(nil)

func (f *ctxCheckingSvc) GetOrCreatePendingForm(ctx context.Context, id int64, formUrl, sub string) (*model.AuditorForm, error) {
	if ctx.Value(tickCtxKey{}) != nil {
		f.sawTickCtx = true
		return nil, errors.New("record processing must not reuse the tick ctx")
	}
	if f.form == nil {
		f.form = &model.AuditorForm{Id: 1, ActivityId: id, Subject: sub, FormUrl: formUrl}
	}
	return f.form, nil
}

func (f *ctxCheckingSvc) ClaimForUpload(ctx context.Context, _ int64) (time.Time, bool, error) {
	if ctx.Value(tickCtxKey{}) != nil {
		f.sawTickCtx = true
		return time.Time{}, false, errors.New("record processing must not reuse the tick ctx")
	}
	return time.Now().Add(time.Minute), true, nil
}

func (f *ctxCheckingSvc) ReleaseClaim(context.Context, int64, time.Time) error { return nil }

func (f *ctxCheckingSvc) UploadForm(ctx context.Context, _ *req.AuditWrapper, _ int64) error {
	if ctx.Value(tickCtxKey{}) != nil {
		f.sawTickCtx = true
		return errors.New("record processing must not reuse the tick ctx")
	}
	f.uploadCnt++
	return nil
}

func (f *ctxCheckingSvc) MarkPushed(ctx context.Context, _ int64, pushedAt time.Time) error {
	if ctx.Value(tickCtxKey{}) != nil {
		f.sawTickCtx = true
		return errors.New("record processing must not reuse the tick ctx")
	}
	now := pushedAt
	f.form.PushedAt = &now
	return nil
}

func (f *ctxCheckingSvc) CreateAuditorForm(context.Context, int64, string, string) (*model.AuditorForm, error) {
	return nil, errors.New("not used")
}

func (f *ctxCheckingSvc) ReconcilePendingForms(context.Context) {}

func (f *ctxCheckingSvc) GetReasons(context.Context, []int64) (map[int64]string, error) {
	return nil, nil
}

func (f *ctxCheckingSvc) PlatformStatus(context.Context, int64) (string, error) {
	return "", errors.New("not used")
}

// TestUpload_IndependentContextPerRecord 整轮 tick 的 ctx（带 tickCtxKey 标记）不得传入单条记录的
// 送审序列；回归点：修复前 uploadPendingForm 复用整轮 ctx，同 tick 内一条挂死会拖垮后续记录。
func TestUpload_IndependentContextPerRecord(t *testing.T) {
	acts := &fakePendingActivities{acts: []model.Activity{{Id: 1001, StudentID: "S1", Title: "t", ActiveForm: "https://form"}}}
	svc := &ctxCheckingSvc{}
	w := newWorkerForTest(acts, svc)

	tickCtx := context.WithValue(context.Background(), tickCtxKey{}, true)
	w.processPendingAuditorActivities(tickCtx)

	if svc.sawTickCtx {
		t.Fatalf("record processing received the tick ctx; it must use an independent ctx")
	}
	if svc.uploadCnt != 1 {
		t.Fatalf("record upload must complete on an independent ctx, got %d uploads", svc.uploadCnt)
	}
}
