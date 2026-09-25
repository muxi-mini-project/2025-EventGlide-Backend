package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/muxi-Infra/auditor-Backend/sdk/v2/client"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// fakeAuditorRepo 记录 Insert 调用次数，用于锁定后台轮询的幂等契约。
type fakeAuditorRepo struct {
	existing      *model.AuditorForm // FindByActivity 的返回值，nil 表示未找到
	insertCnt     int
	lastFormId    int64
	insertDup     *model.AuditorForm  // 非 nil 时 Insert 模拟撞唯一键，并假定并发方已插入该行
	updatedTo     string              // Update 写入的最后一个状态
	pushedPending []model.AuditorForm // FindPushedPending 的返回值
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

func (f *fakeAuditorRepo) Update(_ context.Context, _ int64, status string) error {
	f.updatedTo = status
	return nil
}

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

func (f *fakeAuditorRepo) FindPushedPending(context.Context) ([]model.AuditorForm, error) {
	if f.pushedPending == nil {
		return nil, nil
	}
	return f.pushedPending, nil
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

// TestSyncExistingFormStatus_BigHookID 平台 hook_id 是 int64 雪花值，经 SDK 的
// interface{} 反序列化会被 float64 丢精度。回查必须据此判定"条目已存在"并同步状态，
// 而不是用 id 相等比较（那会永不命中 → 误判不存在 → 继续重传）。
func TestSyncExistingFormStatus_BigHookID(t *testing.T) {
	const bigID int64 = 638278946403647490 // > 2^53，float64 无法精确表示
	platformJSON := `{"msg":"","code":200,"data":{"items":[{"status":0,"hook_id":638278946403647490}]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(platformJSON))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo}

	ok, err := svc.syncExistingFormStatus(context.Background(), bigID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("item with big hook_id should be detected as existing")
	}
	if repo.updatedTo != "pending" {
		t.Fatalf("expected status synced to pending, got %q", repo.updatedTo)
	}
}

// TestSyncExistingFormStatus_EmptyItems 平台返回空 items 时应判定为不存在。
func TestSyncExistingFormStatus_EmptyItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: &fakeAuditorRepo{}}

	ok, err := svc.syncExistingFormStatus(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("empty items should be treated as not existing")
	}
}

// TestReconcilePendingForms_SyncsConclusion 平台已出结论时，对账应把状态落库。
func TestReconcilePendingForms_SyncsConclusion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// status=1 -> Pass
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1,"hook_id":42}]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{pushedPending: []model.AuditorForm{{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"}}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.ReconcilePendingForms(context.Background())

	if repo.updatedTo != "pass" {
		t.Fatalf("expected synced status pass, got %q", repo.updatedTo)
	}
}

// TestReconcilePendingForms_SkipsMissing 平台无该条目时仅告警，不重推、不落库。
func TestReconcilePendingForms_SkipsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{pushedPending: []model.AuditorForm{{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"}}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.ReconcilePendingForms(context.Background())

	if repo.updatedTo != "" {
		t.Fatalf("missing item must not be written back, got %q", repo.updatedTo)
	}
}
