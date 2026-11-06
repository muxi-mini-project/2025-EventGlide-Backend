package dao

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newAuditorDaoForTest(t *testing.T) (*AuditorRepo, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy:         schema.NamingStrategy{SingularTable: true},
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &AuditorRepo{db: gdb, l: logger.NewLoggerSet().Auditor}, mock
}

// TestClaimForUploadSQL 断言占用 SQL 可用且 RowsAffected>0 判定为占用成功。
func TestClaimForUploadSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)
	now := time.Now().Truncate(time.Second)
	lease := now.Add(time.Minute)

	mock.ExpectExec("UPDATE `auditor_form` SET `claimed_at`=").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := repo.ClaimForUpload(context.Background(), 42, now, lease)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected claim to succeed when RowsAffected=1")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestClaimForUploadSQL_ConditionIsGuarded 断言占用 SQL 的 WHERE 条件包含
// pushed_at IS NULL 与租约过期判断——防止有人误删守卫导致无条件占用。
func TestClaimForUploadSQL_ConditionIsGuarded(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)
	now := time.Now().Truncate(time.Second)
	lease := now.Add(time.Minute)

	mock.ExpectExec("WHERE id = \\? AND pushed_at IS NULL AND \\(claimed_at IS NULL OR claimed_at <= \\?\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ok, err := repo.ClaimForUpload(context.Background(), 42, now, lease)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected claim to fail when RowsAffected=0")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseClaimSQL 断言释放按 (id, claimed_at) 精确匹配，避免误清他人占用。
func TestReleaseClaimSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)
	lease := time.Now().Truncate(time.Second).Add(time.Minute)

	mock.ExpectExec("UPDATE `auditor_form` SET `claimed_at`=.*WHERE id = \\? AND claimed_at = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.ReleaseClaim(context.Background(), 42, lease); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindPushedPendingSQL 断言对账查询按 id 游标只取"已推送但平台仍 pending"的表单。
func TestFindPushedPendingSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE pushed_at IS NOT NULL AND status = \\? AND subject <> \\? AND id > \\? ORDER BY id ASC LIMIT \\?").
		WithArgs("pending", "post", int64(10), 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "subject", "status"}).
			AddRow(42, 7, "activity", "pending"))

	forms, err := repo.FindPushedPending(context.Background(), 10, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(forms) != 1 || forms[0].Id != 42 {
		t.Fatalf("expected one form id=42, got %+v", forms)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateIfPendingSkipsTerminal 表单已是终态（pass）时，对账不得覆盖写回。
func TestUpdateIfPendingSkipsTerminal(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(42, "pass"))
	// 不期望任何 UPDATE：条件未命中即返回
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "pending"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateIfPendingUpdatesWhenChanged 表单仍为 pending 且平台给出不同结论时正常写回，
// 并触发 AfterUpdate 推进活动状态。
func TestUpdateIfPendingUpdatesWhenChanged(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "subject", "activity_id"}).
			AddRow(42, "pending", "activity", 1001))
	mock.ExpectExec("UPDATE `auditor_form` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	// AfterUpdate 的 pass 分支：仅当活动仍待审核且无未通过审批时才置为 pass
	mock.ExpectExec("UPDATE activity SET is_checking = 'pass' WHERE id = \\? AND is_checking IN \\('pending_auditor','reject'\\) AND NOT EXISTS \\( SELECT 1 FROM approvement WHERE activity_id = \\? AND stance != 'pass' \\)").
		WithArgs(int64(1001), int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "pass"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateIfPendingSkipsWhenUnchanged 平台结论与现值相同（仍为 pending）时不得写库，
// 避免每轮对账反复刷新 updated_at。
func TestUpdateIfPendingSkipsWhenUnchanged(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "subject"}).AddRow(42, "pending", "activity"))
	// 不期望任何 UPDATE：状态未变化即返回
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "pending"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAuditorFormAfterUpdatePassAllowsReject 平台改判为通过时，已驳回的活动要跟着放行：
// pass 分支的 SQL 必须允许 is_checking 从 reject 迁移，且保留"送审人不全为 pass 则不放行"的守卫。
func TestAuditorFormAfterUpdatePassAllowsReject(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	// 活动已驳回（reject），平台再次回调把表单改成 pass
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "subject", "status"}).
			AddRow(42, 7, "activity", "reject"))
	mock.ExpectExec("UPDATE `auditor_form` SET `status`=\\?,`reason`=\\?,`updated_at`=\\? WHERE `id` = \\?").
		WithArgs("pass", "", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity SET is_checking = 'pass' WHERE id = \\? AND is_checking IN \\('pending_auditor','reject'\\) AND NOT EXISTS \\( SELECT 1 FROM approvement WHERE activity_id = \\? AND stance != 'pass' \\)$").
		WithArgs(int64(7), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 42, "pass", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAuditorFormAfterUpdateRejectOnlyPending 锁定 reject 分支不做对称扩展：
// 仍只允许 pending_auditor -> reject，不把已通过的活动改判下架。
func TestAuditorFormAfterUpdateRejectOnlyPending(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "subject", "status"}).
			AddRow(42, 7, "activity", "pending"))
	mock.ExpectExec("UPDATE `auditor_form` SET `status`=\\?,`reason`=\\?,`updated_at`=\\? WHERE `id` = \\?").
		WithArgs("reject", "", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity SET is_checking = 'reject' WHERE id = \\? AND is_checking = 'pending_auditor'$").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 42, "reject", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateIfPendingMissingIsIdempotent 表单已被撤销删除时，对账更新应幂等成功（与 Update 一致），
// 避免撤销与对账并发时对已删行报错刷日志。
func TestUpdateIfPendingMissingIsIdempotent(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "pass"); err != nil {
		t.Fatalf("missing form on reconcile update must be idempotent success, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindOrphanPostFormsSQL 锁定"待撤销表单"的判据：subject='post' 且对应帖子已不存在（孤儿）。
// 帖子是硬删除，故该判据精确等价于"帖子已被删"，不会误撤活帖子的表单。
func TestFindOrphanPostFormsSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE subject = \\? AND NOT EXISTS \\(SELECT 1 FROM post p WHERE p.id = auditor_form.activity_id\\)").
		WithArgs("post").
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "subject", "pushed_at"}).
			AddRow(9, 1001, "post", nil))

	forms, err := repo.FindOrphanPostForms(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(forms) != 1 || forms[0].Id != 9 {
		t.Fatalf("expected the single orphan form, got %+v", forms)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateAuditorFormMissingIsIdempotent 撤销后表单行已消失，迟到的审核回调应视为幂等成功，
// 而不是报错让平台无休止重试。
func TestUpdateAuditorFormMissingIsIdempotent(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if err := repo.Update(context.Background(), 9, "pass", ""); err != nil {
		t.Fatalf("missing form on update must be idempotent success, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAuditorFormFindByIDSQL 断言按 id 回查表单（撤销前取最新状态）。
func TestAuditorFormFindByIDSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "subject"}).AddRow(9, 1001, "post"))

	form, err := repo.FindByID(context.Background(), 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form.Id != 9 {
		t.Fatalf("expected form 9, got %d", form.Id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateAuditorFormWritesReason 回调带的原因必须与状态一起落库。
func TestUpdateAuditorFormWritesReason(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "activity_id", "status"}).
			AddRow(42, "activity", 7, "pending"))
	mock.ExpectExec("UPDATE `auditor_form` SET `status`=\\?,`reason`=\\?,`updated_at`=\\? WHERE `id` = \\?").
		WithArgs("reject", "资料不全", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 拒绝状态会经 AuditorForm.AfterUpdate 把活动推进为 is_checking='reject'
	mock.ExpectExec("UPDATE activity").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 42, "reject", "资料不全"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateAuditorFormKeepsReasonOnEmptyRejectMsg 驳回回调不带原因时保留已有原因，
// 避免第二次 reject 回调把第一次写入的原因冲成空串。
func TestUpdateAuditorFormKeepsReasonOnEmptyRejectMsg(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "activity_id", "status", "reason"}).
			AddRow(42, "activity", 7, "pending", "资料不全"))
	mock.ExpectExec("UPDATE `auditor_form` SET `status`=\\?,`reason`=\\?,`updated_at`=\\? WHERE `id` = \\?").
		WithArgs("reject", "资料不全", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 42, "reject", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateAuditorFormClearsReasonWhenNotReject 非 reject 状态清空原因：
// 平台把条目重置为 pending 后会保留旧原因，若随后 reject 回调丢失、由对账改回 reject，
// 旧原因会被当成当前原因展示。
func TestUpdateAuditorFormClearsReasonWhenNotReject(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "activity_id", "status", "reason"}).
			AddRow(42, "activity", 7, "reject", "旧原因"))
	mock.ExpectExec("UPDATE `auditor_form` SET `status`=\\?,`reason`=\\?,`updated_at`=\\? WHERE `id` = \\?").
		WithArgs("pending", "", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 42, "pending", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateAuditorFormDoesNotWriteUploadColumns 回调更新只写结论列，不得用读到的旧快照覆盖
// pushed_at / claimed_at：与上传并发时会把已推送表单改回未推送，或误清他人的上传占用。
func TestUpdateAuditorFormDoesNotWriteUploadColumns(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)
	pushedAt := time.Now().Add(-time.Hour)
	claimedAt := time.Now().Add(-time.Minute)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\? ORDER BY `auditor_form`.`id` LIMIT \\?").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "activity_id", "status", "reason", "pushed_at", "claimed_at"}).
			AddRow(42, "activity", 7, "pending", "", pushedAt, claimedAt))
	// SQL 只含 status/reason/updated_at 三列；若带上 pushed_at 或 claimed_at 该断言会失败。
	mock.ExpectExec("UPDATE `auditor_form` SET `status`=\\?,`reason`=\\?,`updated_at`=\\? WHERE `id` = \\?").
		WithArgs("pass", "", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 42, "pass", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindReasonsByActivityIDsSQL 批量查原因只取拒绝状态的活动表单，返回 activity_id -> reason。
func TestFindReasonsByActivityIDsSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT `activity_id`,`reason` FROM `auditor_form` WHERE subject = \\? AND status = \\? AND activity_id IN \\(\\?,\\?\\)").
		WithArgs("activity", "reject", int64(7), int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"activity_id", "reason"}).
			AddRow(7, "资料不全").
			AddRow(8, ""))

	reasons, err := repo.FindReasonsByActivityIDs(context.Background(), []int64{7, 8})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reasons[7] != "资料不全" {
		t.Fatalf("expected reason for 7, got %+v", reasons)
	}
	if reason, ok := reasons[8]; !ok || reason != "" {
		t.Fatalf("expected empty reason for 8 to be present, got %+v", reasons)
	}
	if len(reasons) != 2 {
		t.Fatalf("expected 2 reasons, got %+v", reasons)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindReasonsByActivityIDsEmpty 空 ids 不得发 SQL，直接返回空 map。
func TestFindReasonsByActivityIDsEmpty(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	reasons, err := repo.FindReasonsByActivityIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reasons == nil || len(reasons) != 0 {
		t.Fatalf("expected empty non-nil map, got %+v", reasons)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAuditorFormDeleteSQL 断言按 id 删除本地表单行。
func TestAuditorFormDeleteSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectExec("DELETE FROM `auditor_form` WHERE id = \\?").
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Delete(context.Background(), 9); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
