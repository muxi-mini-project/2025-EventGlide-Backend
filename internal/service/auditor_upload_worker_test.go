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

type fakePendingPosts struct {
	posts []model.Post
	err   error
}

func (f *fakePendingPosts) FindPendingAuditorPosts(context.Context) ([]model.Post, error) {
	return f.posts, f.err
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
	orphans      []model.AuditorForm
	revoked      []int64
	revokeErr    error
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

func (f *fakeAuditorSvc) FindOrphanPostForms(context.Context) ([]model.AuditorForm, error) {
	return f.orphans, nil
}

func (f *fakeAuditorSvc) RevokeForm(_ context.Context, form model.AuditorForm) error {
	f.revoked = append(f.revoked, form.Id)
	return f.revokeErr
}

func newWorkerForTest(posts *fakePendingPosts, svc AuditorService) *AuditorUploadWorker {
	return &AuditorUploadWorker{
		postRepo:       posts,
		auditorService: svc,
		logger:         &logger.LoggerSet{Auditor: zap.NewNop()},
	}
}

// TestPostUpload_IdempotentAcrossTicks 首个 tick 成功送审后，后续 tick 因 form 已推送而跳过，
// 不重复远端上传——这是"回调到达前每 5s 重复送审"的回归点。
func TestPostUpload_IdempotentAcrossTicks(t *testing.T) {
	posts := &fakePendingPosts{posts: []model.Post{{Id: 1001, StudentID: "S1", Title: "t", IsChecking: "checking"}}}
	svc := &fakeAuditorSvc{}
	w := newWorkerForTest(posts, svc)

	w.processPendingAuditorPosts(context.Background())
	if svc.uploadCnt != 1 {
		t.Fatalf("first tick should upload once, got %d", svc.uploadCnt)
	}
	if svc.lastSub != SubjectPost || svc.lastFormUrl != "" {
		t.Fatalf("post form must use subject=%q formUrl=%q, got subject=%q formUrl=%q", SubjectPost, "", svc.lastSub, svc.lastFormUrl)
	}

	w.processPendingAuditorPosts(context.Background())
	if svc.uploadCnt != 1 {
		t.Fatalf("already-pushed form must not be re-uploaded, got %d uploads", svc.uploadCnt)
	}
}

// TestPostUpload_RetriesAfterFailure 上传失败应释放占用，使下个 tick 能立即重试成功。
func TestPostUpload_RetriesAfterFailure(t *testing.T) {
	posts := &fakePendingPosts{posts: []model.Post{{Id: 1001, StudentID: "S1", Title: "t", IsChecking: "checking"}}}
	svc := &fakeAuditorSvc{uploadErr: errors.New("network")}
	w := newWorkerForTest(posts, svc)

	w.processPendingAuditorPosts(context.Background())
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
	w.processPendingAuditorPosts(context.Background())
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
	w := newWorkerForTest(&fakePendingPosts{}, svc)

	w.reconcile(context.Background())
	if svc.reconcileCnt != 1 {
		t.Fatalf("expected reconcile invoked once, got %d", svc.reconcileCnt)
	}
}

// TestProcessPendingAuditorRevokes_RevokesAllOrphans 每个孤儿表单（帖子已删）都应触发一次撤销。
func TestProcessPendingAuditorRevokes_RevokesAllOrphans(t *testing.T) {
	now := time.Now()
	svc := &fakeAuditorSvc{orphans: []model.AuditorForm{
		{Id: 1, Subject: SubjectPost, PushedAt: &now},
		{Id: 2, Subject: SubjectPost},
	}}
	w := newWorkerForTest(&fakePendingPosts{}, svc)

	w.processPendingAuditorRevokes(context.Background())

	if len(svc.revoked) != 2 || svc.revoked[0] != 1 || svc.revoked[1] != 2 {
		t.Fatalf("expected both orphan forms revoked, got %v", svc.revoked)
	}
}

// TestProcessPendingAuditorRevokes_ContinuesAfterFailure 单个撤销失败不应中断整轮，
// 且失败项保留在孤儿集合里由下一轮重试。
func TestProcessPendingAuditorRevokes_ContinuesAfterFailure(t *testing.T) {
	svc := &fakeAuditorSvc{
		orphans:   []model.AuditorForm{{Id: 1}, {Id: 2}},
		revokeErr: errors.New("auditor down"),
	}
	w := newWorkerForTest(&fakePendingPosts{}, svc)

	w.processPendingAuditorRevokes(context.Background())
	if len(svc.revoked) != 2 {
		t.Fatalf("worker should attempt every orphan even if one fails, got %v", svc.revoked)
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

func (f *ctxCheckingSvc) FindOrphanPostForms(context.Context) ([]model.AuditorForm, error) {
	return nil, nil
}

func (f *ctxCheckingSvc) RevokeForm(context.Context, model.AuditorForm) error { return nil }

// blockingUploadSvc 让上传挂起直到 ctx 取消，用于验证 Stop 能取消在途送审。
type blockingUploadSvc struct {
	*fakeAuditorSvc
	entered  chan struct{}
	canceled chan struct{}
}

func (f *blockingUploadSvc) UploadForm(ctx context.Context, _ *req.AuditWrapper, _ int64) error {
	f.entered <- struct{}{}
	<-ctx.Done()
	f.canceled <- struct{}{}
	return ctx.Err()
}

// TestAuditorWorkerStopCancelsUpload Stop 必须取消在途上传并有界返回；未 Start 时 Stop 是空操作。
func TestAuditorWorkerStopCancelsUpload(t *testing.T) {
	oldInterval, oldStop := auditorUploadInterval, auditorStopTimeout
	auditorUploadInterval = 10 * time.Millisecond
	auditorStopTimeout = 2 * time.Second
	t.Cleanup(func() { auditorUploadInterval, auditorStopTimeout = oldInterval, oldStop })

	svc := &blockingUploadSvc{
		fakeAuditorSvc: &fakeAuditorSvc{},
		entered:        make(chan struct{}, 1),
		canceled:       make(chan struct{}, 1),
	}
	posts := &fakePendingPosts{posts: []model.Post{{Id: 1001, StudentID: "S1"}}}
	w := newWorkerForTest(posts, svc)

	// 未 Start 时 Stop 是幂等空操作。
	newWorkerForTest(posts, &fakeAuditorSvc{}).Stop()

	w.Start(context.Background())
	select {
	case <-svc.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker 未发起送审上传")
	}

	start := time.Now()
	w.Stop()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Stop 等待退出耗时 %v, want 1s 内", elapsed)
	}
	select {
	case <-svc.canceled:
	case <-time.After(time.Second):
		t.Fatal("在途上传未收到 cancel")
	}
	w.Stop() // 重复 Stop 幂等
}

// TestUpload_IndependentContextPerRecord 整轮 tick 的 ctx（带 tickCtxKey 标记）不得传入单条记录的
// 送审序列；回归点：修复前 uploadPendingForm 复用整轮 ctx，同 tick 内一条挂死会拖垮后续记录。
func TestUpload_IndependentContextPerRecord(t *testing.T) {
	posts := &fakePendingPosts{posts: []model.Post{{Id: 1001, StudentID: "S1", Title: "t", IsChecking: "checking"}}}
	svc := &ctxCheckingSvc{}
	w := newWorkerForTest(posts, svc)

	tickCtx := context.WithValue(context.Background(), tickCtxKey{}, true)
	w.processPendingAuditorPosts(tickCtx)

	if svc.sawTickCtx {
		t.Fatalf("record processing received the tick ctx; it must use an independent ctx")
	}
	if svc.uploadCnt != 1 {
		t.Fatalf("record upload must complete on an independent ctx, got %d uploads", svc.uploadCnt)
	}
}
