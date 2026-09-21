package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"gorm.io/gorm"
)

// fakeAuditorRepo 记录 Insert 调用次数，用于锁定后台轮询的幂等契约。
type fakeAuditorRepo struct {
	existing   *model.AuditorForm // FindByActivity 的返回值，nil 表示未找到
	insertCnt  int
	lastFormId int64
	insertDup  *model.AuditorForm // 非 nil 时 Insert 模拟撞唯一键，并假定并发方已插入该行
}

var _ dao.AuditorRepository = (*fakeAuditorRepo)(nil)

func (f *fakeAuditorRepo) Insert(_ context.Context, activityId int64, formUrl string, sub string) (*model.AuditorForm, error) {
	f.insertCnt++
	if f.insertDup != nil {
		f.existing = f.insertDup
		return nil, gorm.ErrDuplicatedKey
	}
	form := &model.AuditorForm{
		Id:         int64(1000 + f.insertCnt),
		ActivityId: activityId,
		Subject:    sub,
		FormUrl:    formUrl,
		Status:     "pending",
	}
	f.existing = form
	return form, nil
}

func (f *fakeAuditorRepo) Update(context.Context, int64, string) error { return nil }

func (f *fakeAuditorRepo) Get(context.Context, int64) (model.AuditorForm, error) {
	return model.AuditorForm{}, nil
}

func (f *fakeAuditorRepo) IsRejected(context.Context, int64) (bool, error) { return false, nil }

func (f *fakeAuditorRepo) FindByActivity(_ context.Context, _ int64, _ string) (model.AuditorForm, error) {
	if f.existing == nil {
		return model.AuditorForm{}, gorm.ErrRecordNotFound
	}
	return *f.existing, nil
}

func (f *fakeAuditorRepo) MarkPushed(_ context.Context, formId int64, pushedAt time.Time) error {
	if f.existing != nil {
		f.existing.PushedAt = &pushedAt
	}
	f.lastFormId = formId
	return nil
}

func (f *fakeAuditorRepo) ClaimForUpload(_ context.Context, formId int64, now, leaseUntil time.Time) (bool, error) {
	if f.existing == nil || f.existing.PushedAt != nil {
		return false, nil
	}
	if f.existing.ClaimedAt != nil && f.existing.ClaimedAt.After(now) {
		return false, nil // 已被持有且租约未过期
	}
	f.existing.ClaimedAt = &leaseUntil
	return true, nil
}

func (f *fakeAuditorRepo) ReleaseClaim(_ context.Context, formId int64, leaseUntil time.Time) error {
	if f.existing != nil && f.existing.ClaimedAt != nil && f.existing.ClaimedAt.Equal(leaseUntil) {
		f.existing.ClaimedAt = nil
	}
	return nil
}

func newAuditorServiceForTest(repo dao.AuditorRepository) *auditorService {
	return &auditorService{AuditorRepo: repo, l: nil}
}

// TestGetOrCreatePendingForm_NoFormInserts 没有表单时应新建。
func TestGetOrCreatePendingForm_NoFormInserts(t *testing.T) {
	repo := &fakeAuditorRepo{}
	svc := newAuditorServiceForTest(repo)

	form, err := svc.GetOrCreatePendingForm(context.Background(), 1, "https://form", SubjectActivity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form == nil || repo.insertCnt != 1 {
		t.Fatalf("expected 1 insert, got %d (form=%v)", repo.insertCnt, form)
	}
}

// TestGetOrCreatePendingForm_UnpushedReuses 存在未推送表单时应复用，不新建。
func TestGetOrCreatePendingForm_UnpushedReuses(t *testing.T) {
	repo := &fakeAuditorRepo{existing: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity}}
	svc := newAuditorServiceForTest(repo)

	form, err := svc.GetOrCreatePendingForm(context.Background(), 1, "https://form", SubjectActivity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form.Id != 42 || repo.insertCnt != 0 {
		t.Fatalf("expected reuse of form 42 with 0 inserts, got id=%d inserts=%d", form.Id, repo.insertCnt)
	}
}

// TestGetOrCreatePendingForm_PushedSkips 已推送成功的表单应跳过，既不新建也不返回表单。
// 这是"回调到达前 worker 每 5s 重复送审"的核心回归点。
func TestGetOrCreatePendingForm_PushedSkips(t *testing.T) {
	now := time.Now()
	repo := &fakeAuditorRepo{existing: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity, PushedAt: &now}}
	svc := newAuditorServiceForTest(repo)

	form, err := svc.GetOrCreatePendingForm(context.Background(), 1, "https://form", SubjectActivity)
	if !errors.Is(err, ErrFormAlreadyPushed) {
		t.Fatalf("expected ErrFormAlreadyPushed, got %v", err)
	}
	if form != nil {
		t.Fatalf("expected nil form when already pushed, got %v", form)
	}
	if repo.insertCnt != 0 {
		t.Fatalf("expected 0 inserts when already pushed, got %d", repo.insertCnt)
	}
}

// TestGetOrCreatePendingForm_DupKeyReuses 并发下 Insert 撞唯一键时，应回读已存在行并复用，
// 而不是把错误抛给调用方（多实例场景的兜底）。
func TestGetOrCreatePendingForm_DupKeyReuses(t *testing.T) {
	repo := &fakeAuditorRepo{insertDup: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity}}
	svc := newAuditorServiceForTest(repo)

	form, err := svc.GetOrCreatePendingForm(context.Background(), 1, "https://form", SubjectActivity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form == nil || form.Id != 42 {
		t.Fatalf("expected reused form 42, got %v", form)
	}
	if repo.insertCnt != 1 {
		t.Fatalf("expected 1 insert attempt, got %d", repo.insertCnt)
	}
}

// TestGetOrCreatePendingForm_DupKeyPushedSkips 并发撞键且已存在行已推送时，应返回 ErrFormAlreadyPushed。
func TestGetOrCreatePendingForm_DupKeyPushedSkips(t *testing.T) {
	now := time.Now()
	repo := &fakeAuditorRepo{insertDup: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity, PushedAt: &now}}
	svc := newAuditorServiceForTest(repo)

	form, err := svc.GetOrCreatePendingForm(context.Background(), 1, "https://form", SubjectActivity)
	if !errors.Is(err, ErrFormAlreadyPushed) {
		t.Fatalf("expected ErrFormAlreadyPushed, got %v", err)
	}
	if form != nil {
		t.Fatalf("expected nil form, got %v", form)
	}
	if repo.insertCnt != 1 {
		t.Fatalf("expected 1 insert attempt, got %d", repo.insertCnt)
	}
}

// TestClaimForUpload_HeldLeaseBlocksSecondClaim 第一个执行者占用后，租约未过期时第二个不能占用。
// 这是"两个 worker 同时读到同一未推送表单、各自重复上传"的回归点。
func TestClaimForUpload_HeldLeaseBlocksSecondClaim(t *testing.T) {
	repo := &fakeAuditorRepo{existing: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity}}
	first := newAuditorServiceForTest(repo)
	second := newAuditorServiceForTest(repo)

	_, ok, err := first.ClaimForUpload(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("first claimant should acquire the lease")
	}
	_, ok, err = second.ClaimForUpload(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("second claimant must not acquire while lease is held")
	}
}

// TestClaimForUpload_ExpiredLeaseAllowsReclaim 租约过期后应可重新占用（避免永久卡死）。
func TestClaimForUpload_ExpiredLeaseAllowsReclaim(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	repo := &fakeAuditorRepo{existing: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity, ClaimedAt: &past}}
	svc := newAuditorServiceForTest(repo)

	_, ok, err := svc.ClaimForUpload(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expired lease should be reclaimable")
	}
}

// TestReleaseClaim_FreesForRetry 上传失败释放占用后，下一次可立即重新占用。
func TestReleaseClaim_FreesForRetry(t *testing.T) {
	repo := &fakeAuditorRepo{existing: &model.AuditorForm{Id: 42, ActivityId: 1, Subject: SubjectActivity}}
	svc := newAuditorServiceForTest(repo)

	lease, ok, err := svc.ClaimForUpload(context.Background(), 42)
	if err != nil || !ok {
		t.Fatalf("expected claim, got ok=%v err=%v", ok, err)
	}
	if err := svc.ReleaseClaim(context.Background(), 42, lease); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok, err = svc.ClaimForUpload(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("should be able to reclaim after release")
	}
}
