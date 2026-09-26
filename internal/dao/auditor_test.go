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

	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE pushed_at IS NOT NULL AND status = \\? AND id > \\? ORDER BY id ASC LIMIT \\?").
		WithArgs("pending", int64(10), 100).
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

// TestUpdateIfPendingUpdatesWhenPending 表单仍为 pending 时正常写回。
func TestUpdateIfPendingUpdatesWhenPending(t *testing.T) {
	repo, mock := newAuditorDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `auditor_form` WHERE id = \\?.*FOR UPDATE").
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "subject"}).AddRow(42, "pending", "activity"))
	// 用 pending 目标状态，避免触发 AfterUpdate 对 activity/post 的写操作
	mock.ExpectExec("UPDATE `auditor_form` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.UpdateIfPending(context.Background(), 42, "pending"); err != nil {
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

	if err := repo.Update(context.Background(), 9, "pass"); err != nil {
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
