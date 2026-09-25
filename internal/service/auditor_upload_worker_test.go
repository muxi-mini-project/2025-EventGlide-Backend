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

	w.reconcile()
	if svc.reconcileCnt != 1 {
		t.Fatalf("expected reconcile invoked once, got %d", svc.reconcileCnt)
	}
}
