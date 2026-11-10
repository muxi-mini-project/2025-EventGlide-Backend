package dao

import (
	"bytes"
	"context"
	"log"
	"strings"
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

// TestFindPushedPendingSQL 断言对账查询按 id 游标只取"已推送但平台仍 pending"且未被撤销的表单。
func TestFindPushedPendingSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE \\(pushed_at IS NOT NULL AND status = \\? AND subject <> \\? AND revoked_at IS NULL AND id > \\?\\) AND \\(EXISTS \\(SELECT 1 FROM activity a WHERE a.id = auditor_form.activity_id AND a.end_time > \\?\\)\\) ORDER BY id ASC LIMIT \\?").
		WithArgs("pending", "post", int64(10), sqlmock.AnyArg(), 100).
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

// TestFindExpiredPushedPendingSQL 断言撤销查询只取 subject=activity、仍 pending、未被撤销且所属活动已过期的表单。
func TestFindExpiredPushedPendingSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE \\(subject = \\? AND status = \\? AND pushed_at IS NOT NULL AND revoked_at IS NULL AND id > \\?\\) AND \\(EXISTS \\(SELECT 1 FROM activity a WHERE a.id = auditor_form.activity_id AND a.end_time <= \\?\\)\\) ORDER BY id ASC LIMIT \\?").
		WithArgs("activity", "pending", int64(10), sqlmock.AnyArg(), 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "subject", "status"}).
			AddRow(42, 7, "activity", "pending"))

	forms, err := repo.FindExpiredPushedPending(context.Background(), 10, 50)
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

// TestMarkRevokedSQL 断言撤销标记只写 revoked_at 一列，且以 revoked_at IS NULL 保证幂等。
func TestMarkRevokedSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)
	now := time.Now()

	mock.ExpectExec("UPDATE `auditor_form` SET `revoked_at`=\\? WHERE id = \\? AND revoked_at IS NULL").
		WithArgs(now, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.MarkRevoked(context.Background(), 42, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

// TestUpdateIfPendingClearsReasonForNonReject 对账写回非 reject 结论时必须清空旧的驳回原因：
// 否则"驳回(有原因) -> 回查无法确认退回 pending（保留原因） -> 对账写回 pass"之后旧原因残留，
// 后续不带原因的 reject 回调会把它当成新的驳回原因展示。
func TestUpdateIfPendingClearsReasonForNonReject(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "subject", "activity_id", "reason", "form_url"}).
			AddRow(42, "pending", "activity", 1001, "旧原因", ""))
	mock.ExpectExec("UPDATE `auditor_form` SET").
		WithArgs("activity", int64(1001), "pass", "", "", nil, nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity SET is_checking = 'pass'").
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

// captureStdLog 把标准库 log 输出重定向到返回的 buffer，供断言 AfterUpdate 的告警。
// 依赖本包测试不使用 t.Parallel：log.SetOutput 是进程级全局状态，并行用例会互相
// 覆盖输出、造成跨用例串扰。
func captureStdLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return &buf
}

// TestUpdateIfPendingNoWarnWhenActivityAlreadyReject 退回 pending 的表单随后由对账写回 reject 时，
// 活动通常已是 reject（条件更新命中 0 行），属同结论写入，AfterUpdate 不应再打 WARN，
// 否则这条正常路径会持续刷假告警。
func TestUpdateIfPendingNoWarnWhenActivityAlreadyReject(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	buf := captureStdLog(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "subject", "activity_id"}).
			AddRow(42, "pending", "activity", 1001))
	mock.ExpectExec("UPDATE `auditor_form` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity SET is_checking = 'reject' WHERE id = \\? AND is_checking = 'pending_auditor'$").
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COALESCE\\(is_checking, ''\\) FROM `activity` WHERE id = \\?").
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"is_checking"}).AddRow("reject"))
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "reject"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "WARN") {
		t.Fatalf("same-conclusion reconcile write must stay quiet, got log: %s", buf.String())
	}
}

// TestUpdateIfPendingWarnsWhenActivityMissing 活动查不到时 cur 为空串，
// 说明结论没有真正落到活动上，AfterUpdate 必须告警而不是静默当成同结论。
func TestUpdateIfPendingWarnsWhenActivityMissing(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	buf := captureStdLog(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "subject", "activity_id"}).
			AddRow(42, "pending", "activity", 1001))
	mock.ExpectExec("UPDATE `auditor_form` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE activity SET is_checking = 'reject' WHERE id = \\? AND is_checking = 'pending_auditor'$").
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// 活动不存在：查询命中 0 行，cur 保持空串
	mock.ExpectQuery("SELECT COALESCE\\(is_checking, ''\\) FROM `activity` WHERE id = \\?").
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"is_checking"}))
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "reject"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `is_checking=""`) {
		t.Fatalf("missing activity must WARN with empty is_checking, got log: %s", buf.String())
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

// TestUpdateIfPendingMissingIsIdempotent 表单行已被删除时，对账更新应幂等成功（与 Update 一致），
// 避免对已删行报错刷日志。
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

// TestRevertRejectToPendingSQL 条件命中时只写 status（updated_at 由 GORM 自动维护），
// 不触碰 reason/pushed_at/claimed_at，避免回查抖动把已驳回活动的驳回原因清空，
// 也避免覆盖并发写入的推送/占用信息。
func TestRevertRejectToPendingSQL(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectExec("^UPDATE `auditor_form` SET `status`=\\?,`updated_at`=\\? WHERE id = \\? AND status = \\?$").
		WithArgs("pending", sqlmock.AnyArg(), int64(42), "reject").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.RevertRejectToPending(context.Background(), 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRevertRejectToPendingSkipsWhenNotReject 条件未命中（状态已被其它路径推进）时不写行，
// 并视为幂等成功。
func TestRevertRejectToPendingSkipsWhenNotReject(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectExec("^UPDATE `auditor_form` SET `status`=\\?,`updated_at`=\\? WHERE id = \\? AND status = \\?$").
		WithArgs("pending", sqlmock.AnyArg(), int64(42), "reject").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.RevertRejectToPending(context.Background(), 42); err != nil {
		t.Fatalf("rows affected=0 must be idempotent success, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateAuditorFormMissingIsIdempotent 表单行已消失时，迟到的审核回调应视为幂等成功，
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

// TestAuditorFormFindByIDSQL 断言按 id 回查表单。
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
