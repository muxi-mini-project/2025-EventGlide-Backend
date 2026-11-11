package repo

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newActivityRepoForTest(t *testing.T) (*ActivityRepo, sqlmock.Sqlmock) {
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

	actDao := dao.NewActDao(gdb, &config.Conf{}, logger.NewLoggerSet())
	return &ActivityRepo{dao: actDao}, mock
}

func newCreateAct() *model.Activity {
	return &model.Activity{
		Id:         tools.MustGenerateID(),
		StudentID:  "S20250001",
		Title:      "新建活动",
		IsChecking: "pending_signers",
		StartTime:  "2026-12-01 10:00:00",
		EndTime:    "2026-12-01 12:00:00",
	}
}

// TestCreateActivityClearsDraft 普通创建活动仍要清掉该学生的活动草稿（草稿转正式）。
func TestCreateActivityClearsDraft(t *testing.T) {
	r, mock := newActivityRepoForTest(t)
	act := newCreateAct()

	mock.ExpectQuery("SELECT `id` FROM `activity_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("DELETE FROM `activity_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `activity`").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := r.CreateActivity(context.Background(), r.dao.DB(), act, nil, "S20250001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCreateActivityKeepDraftSkipsCleanup 重新送审走同一创建逻辑，但不清草稿：
// 用户可能正在新建另一条活动。
func TestCreateActivityKeepDraftSkipsCleanup(t *testing.T) {
	r, mock := newActivityRepoForTest(t)
	act := newCreateAct()

	mock.ExpectExec("INSERT INTO `activity`").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := r.CreateActivityKeepDraft(context.Background(), r.dao.DB(), act, nil, "S20250001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
