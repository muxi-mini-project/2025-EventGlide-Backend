package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
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
	existing       *model.AuditorForm // FindByActivity 的返回值，nil 表示未找到
	byID           *model.AuditorForm // FindByID 的返回值，nil 表示未找到
	findErr        error              // 非 nil 时 FindByID 返回该错误
	insertCnt      int
	lastFormId     int64
	insertDup      *model.AuditorForm  // 非 nil 时 Insert 模拟撞唯一键，并假定并发方已插入该行
	updatedTo      string              // Update 写入的最后一个状态
	updatedReason  string              // Update 写入的最后一个原因
	revertCnt      int                 // RevertRejectToPending 调用次数
	revertFormId   int64               // RevertRejectToPending 收到的表单 id
	reasons        map[int64]string    // FindReasonsByActivityIDs 的返回值
	pushedPending  []model.AuditorForm // FindPushedPending 的返回值
	expiredForms   []model.AuditorForm // FindExpiredPushedPending 的返回值
	markRevokedErr error               // 非 nil 时 MarkRevoked 返回该错误
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

func (f *fakeAuditorRepo) Update(_ context.Context, _ int64, status string, reason string) error {
	f.updatedTo = status
	f.updatedReason = reason
	return nil
}

func (f *fakeAuditorRepo) FindReasonsByActivityIDs(_ context.Context, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		if reason, ok := f.reasons[id]; ok {
			out[id] = reason
		}
	}
	return out, nil
}

func (f *fakeAuditorRepo) UpdateIfPending(_ context.Context, _ int64, status string) error {
	f.updatedTo = status
	return nil
}

// RevertRejectToPending 模拟条件更新：真实 DAO 只在 status='reject' 时命中。
func (f *fakeAuditorRepo) RevertRejectToPending(_ context.Context, formId int64) error {
	f.revertCnt++
	f.revertFormId = formId
	return nil
}

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

func (f *fakeAuditorRepo) FindPushedPending(_ context.Context, afterID int64, limit int) ([]model.AuditorForm, error) {
	forms := append([]model.AuditorForm(nil), f.pushedPending...)
	sort.Slice(forms, func(i, j int) bool { return forms[i].Id < forms[j].Id })
	var out []model.AuditorForm
	for _, form := range forms {
		if form.Id > afterID {
			out = append(out, form)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeAuditorRepo) FindExpiredPushedPending(_ context.Context, afterID int64, limit int) ([]model.AuditorForm, error) {
	forms := append([]model.AuditorForm(nil), f.expiredForms...)
	sort.Slice(forms, func(i, j int) bool { return forms[i].Id < forms[j].Id })
	var out []model.AuditorForm
	for _, form := range forms {
		// 与真实查询一致：已撤销的行不再返回。
		if form.RevokedAt == nil && form.Id > afterID {
			out = append(out, form)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkRevoked 模拟写入 revoked_at：只更新内存中的过期表单副本，供后续查询过滤。
func (f *fakeAuditorRepo) MarkRevoked(_ context.Context, formId int64, revokedAt time.Time) error {
	if f.markRevokedErr != nil {
		return f.markRevokedErr
	}
	for i := range f.expiredForms {
		if f.expiredForms[i].Id == formId {
			f.expiredForms[i].RevokedAt = &revokedAt
			return nil
		}
	}
	return nil
}

// revokedForm 返回内存中该表单的 revoked_at，nil 表示未被标记撤销。
func (f *fakeAuditorRepo) revokedForm(formId int64) *time.Time {
	for i := range f.expiredForms {
		if f.expiredForms[i].Id == formId {
			return f.expiredForms[i].RevokedAt
		}
	}
	return nil
}

func (f *fakeAuditorRepo) ReleaseClaim(_ context.Context, formId int64, leaseUntil time.Time) error {
	if f.existing != nil && f.existing.ClaimedAt != nil && f.existing.ClaimedAt.Equal(leaseUntil) {
		f.existing.ClaimedAt = nil
	}
	return nil
}

func (f *fakeAuditorRepo) FindByID(_ context.Context, formId int64) (model.AuditorForm, error) {
	if f.findErr != nil {
		return model.AuditorForm{}, f.findErr
	}
	if f.byID == nil || f.byID.Id != formId {
		return model.AuditorForm{}, gorm.ErrRecordNotFound
	}
	return *f.byID, nil
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

// TestSyncExistingFormStatus_MultipleItems 平台返回多条时无法确认哪条对应本表单：
// 按"存在"处理以止住重传，但不得写库。
func TestSyncExistingFormStatus_MultipleItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1},{"status":2}]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	ok, err := svc.syncExistingFormStatus(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("multiple items should be treated as existing")
	}
	if repo.updatedTo != "" {
		t.Fatalf("multiple items must not be written back, got %q", repo.updatedTo)
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

// TestReconcilePendingForms_CursorWrapsAfterShortBatch 不足一批（已到尾部）时游标应回绕到 0。
func TestReconcilePendingForms_CursorWrapsAfterShortBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{pushedPending: []model.AuditorForm{{Id: 42, Subject: SubjectActivity, Status: "pending"}}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.ReconcilePendingForms(context.Background())

	if svc.reconcileCursor != 0 {
		t.Fatalf("cursor should wrap to 0 after a short batch, got %d", svc.reconcileCursor)
	}
}

func newAuditorServiceWithServer(t *testing.T, handler http.HandlerFunc) (*auditorService, *fakeAuditorRepo) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{}
	return &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}, repo
}

// TestRevokeExpiredForms_RevokesOnlyPending 只有平台仍 pending 的过期条目才撤销；
// 平台已出结论（pass/reject）的不删，直接标记撤销完成。本地表单行一律保留。
func TestRevokeExpiredForms_RevokesOnlyPending(t *testing.T) {
	var mu sync.Mutex
	var deleted []string
	queryCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			queryCnt++
			mu.Unlock()
			ids := r.URL.Query().Get("ids")
			status := 0 // pending
			if ids == "43" {
				status = 1 // pass：平台已出结论，不应撤销
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"msg":"","code":200,"data":{"items":[{"status":%d,"hook_id":%s}]}}`, status, ids)))
		case http.MethodDelete:
			mu.Lock()
			deleted = append(deleted, r.URL.Path)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":42}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
		{Id: 43, ActivityId: 8, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	if len(deleted) != 1 || !strings.HasSuffix(deleted[0], "/42") {
		mu.Unlock()
		t.Fatalf("only the still-pending form should be deleted, got %v", deleted)
	}
	queriesAfterFirstRound := queryCnt
	mu.Unlock()

	// 两条都已写入 revoked_at：撤销过的与已出结论的都不再参与下一轮扫描。
	if repo.revokedForm(42) == nil {
		t.Fatalf("deleted form must have revoked_at set")
	}
	if repo.revokedForm(43) == nil {
		t.Fatalf("concluded form must have revoked_at set")
	}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if queryCnt != queriesAfterFirstRound {
		t.Fatalf("revoked rows must not be scanned again, extra platform queries=%d", queryCnt-queriesAfterFirstRound)
	}
	if len(deleted) != 1 {
		t.Fatalf("revoked rows must not be deleted again, got %v", deleted)
	}
}

// TestRevokeExpiredForms_KeepsOnPlatformFailure 平台撤销失败时不能标记完成，下一轮应重试。
func TestRevokeExpiredForms_KeepsOnPlatformFailure(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":0}]}}`))
		case http.MethodDelete:
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())
	if repo.revokedForm(42) != nil {
		t.Fatalf("failed delete must not set revoked_at")
	}
	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if deleteCnt != 2 {
		t.Fatalf("failed form should be retried next round, delete calls=%d", deleteCnt)
	}
}

// TestRevokeExpiredForms_ItemMissingTreatedAsSuccess 平台上已无该条目视为已撤销，不再发删除请求。
func TestRevokeExpiredForms_ItemMissingTreatedAsSuccess(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":42}`))
			return
		}
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if deleteCnt != 0 {
		t.Fatalf("item already gone must not trigger delete, got %d calls", deleteCnt)
	}
	if repo.revokedForm(42) == nil {
		t.Fatalf("item already gone should set revoked_at")
	}
}

// TestRevokeExpiredForms_DeleteErrorButItemGone 删除请求报错（如条目已不存在导致 4xx）时，
// 回查确认平台已无该条目即按撤销成功处理，避免每小时重试。
func TestRevokeExpiredForms_DeleteErrorButItemGone(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`not found`))
			return
		}
		mu.Lock()
		gone := deleteCnt > 0
		mu.Unlock()
		if gone {
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":0}]}}`))
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())

	if repo.revokedForm(42) == nil {
		t.Fatalf("item confirmed gone after delete error should set revoked_at")
	}
}

// TestRevokeExpiredForms_DeleteErrorButPlatformConcluded 删除失败后回查发现平台已出结论
// （审核已结束），条目已不需要撤销：标记完成，下一轮不再扫描。
func TestRevokeExpiredForms_DeleteErrorButPlatformConcluded(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	queryCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		default:
			mu.Lock()
			concluded := deleteCnt > 0
			queryCnt++
			mu.Unlock()
			if concluded {
				// 删除失败后回查：平台已给出 pass 结论。
				_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":1}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":0}]}}`))
		}
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	queriesAfterFirstRound := queryCnt
	mu.Unlock()
	if repo.revokedForm(42) == nil {
		t.Fatalf("platform concluded after delete attempt should set revoked_at")
	}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if queryCnt != queriesAfterFirstRound {
		t.Fatalf("revoked rows must not be scanned again, extra platform queries=%d", queryCnt-queriesAfterFirstRound)
	}
	if deleteCnt != 1 {
		t.Fatalf("revoked rows must not be deleted again, delete calls=%d", deleteCnt)
	}
}

// TestRevokeExpiredForms_DeleteErrorStatusUnknownRetried 删除失败后回查也失败（网络/非法响应）时，
// 无法确认条目是否还在，不能标记完成，下一轮必须重试。
func TestRevokeExpiredForms_DeleteErrorStatusUnknownRetried(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	queryCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		default:
			mu.Lock()
			queryCnt++
			n := queryCnt
			mu.Unlock()
			// 奇数号为撤销前的状态查询（仍 pending），偶数号为删除失败后的回查（异常）。
			if n%2 == 0 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`boom`))
				return
			}
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":0}]}}`))
		}
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())
	if repo.revokedForm(42) != nil {
		t.Fatalf("unknown platform status must not set revoked_at")
	}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if repo.revokedForm(42) != nil {
		t.Fatalf("unknown platform status must not set revoked_at on retry")
	}
	if deleteCnt != 2 {
		t.Fatalf("unknown platform status should retry next round, delete calls=%d", deleteCnt)
	}
}

// TestRevokeExpiredForms_DeleteResponseShapeErrorConfirmedGone 平台以 200 + 非预期 data 形状
// 返回删除结果时，SDK 会判为业务失败；回查确认条目已不在平台后仍应视为撤销成功，避免每小时重试。
func TestRevokeExpiredForms_DeleteResponseShapeErrorConfirmedGone(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":null}`))
		default:
			// 首次回查仍 pending（删除前），删除后的回查返回空集。
			mu.Lock()
			gone := deleteCnt > 0
			mu.Unlock()
			if gone {
				_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":0}]}}`))
		}
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{expiredForms: []model.AuditorForm{
		{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"},
	}}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if deleteCnt != 1 {
		t.Fatalf("expected exactly one delete attempt, got %d", deleteCnt)
	}
	if repo.revokedForm(42) == nil {
		t.Fatalf("item confirmed gone after delete must set revoked_at")
	}
}

// TestRevokeExpiredForms_MarkRevokedFailureRetried 撤销成功但写 revoked_at 失败时，
// 该行不会被隐藏，下一轮会再次扫描并补写标记（平台条目缺失时不会重复删除）。
func TestRevokeExpiredForms_MarkRevokedFailureRetried(t *testing.T) {
	var mu sync.Mutex
	deleteCnt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			mu.Lock()
			deleteCnt++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":42}`))
		default:
			mu.Lock()
			gone := deleteCnt > 0
			mu.Unlock()
			if gone {
				_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[{"status":0}]}}`))
		}
	}))
	defer srv.Close()

	cli, err := client.NewClient(client.Config{ApiKey: "k", Region: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeAuditorRepo{
		expiredForms:   []model.AuditorForm{{Id: 42, ActivityId: 7, Subject: SubjectActivity, Status: "pending"}},
		markRevokedErr: errors.New("db down"),
	}
	svc := &auditorService{MuxiCli: cli, AuditorRepo: repo, l: zap.NewNop()}

	svc.RevokeExpiredForms(context.Background())
	if repo.revokedForm(42) != nil {
		t.Fatalf("mark failure must not persist revoked_at")
	}

	repo.markRevokedErr = nil
	svc.RevokeExpiredForms(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if repo.revokedForm(42) == nil {
		t.Fatalf("retry should persist revoked_at")
	}
	if deleteCnt != 1 {
		t.Fatalf("platform item already gone, must not delete twice, got %d", deleteCnt)
	}
}

// TestPlatformStatusMissingIsSentinel 平台无条目时返回可判定的哨兵错误，撤销流程据此区分"已无需撤销"。
func TestPlatformStatusMissingIsSentinel(t *testing.T) {
	svc, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"msg":"","code":200,"data":{"items":[]}}`))
	})

	_, err := svc.PlatformStatus(context.Background(), 42)
	if !errors.Is(err, ErrAuditorItemNotFound) {
		t.Fatalf("expected ErrAuditorItemNotFound, got %v", err)
	}
}

// TestPlatformStatus 平台状态映射与条目数校验：仅当恰好一条时才采信，否则报错。
func TestPlatformStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
		wantError  bool
	}{
		{name: "pending", body: `{"msg":"","code":200,"data":{"items":[{"status":0}]}}`, want: "pending"},
		{name: "pass", body: `{"msg":"","code":200,"data":{"items":[{"status":1}]}}`, want: "pass"},
		{name: "reject", body: `{"msg":"","code":200,"data":{"items":[{"status":2}]}}`, want: "reject"},
		{name: "unknown status", body: `{"msg":"","code":200,"data":{"items":[{"status":99}]}}`, want: ""},
		{name: "empty items", body: `{"msg":"","code":200,"data":{"items":[]}}`, wantError: true},
		{name: "two items", body: `{"msg":"","code":200,"data":{"items":[{"status":1},{"status":2}]}}`, wantError: true},
		{name: "data type illegal", body: `{"msg":"","code":200,"data":"boom"}`, wantError: true},
		{name: "http 500", statusCode: http.StatusInternalServerError, body: `boom`, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newAuditorServiceWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				}
				_, _ = w.Write([]byte(tt.body))
			})

			got, err := svc.PlatformStatus(context.Background(), 42)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got status %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected status %q, got %q", tt.want, got)
			}
		})
	}
}
