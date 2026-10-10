package dao

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newActDaoForTest(t *testing.T) (*ActDao, sqlmock.Sqlmock) {
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

	return &ActDao{db: gdb, l: logger.NewLoggerSet().Activity}, mock
}

// TestDeleteDraftsByStudentIDRemovesImages 删除活动草稿须一并清理 activity_draft 图片，
// 否则草稿「先删后建」会不断残留孤儿 image 行。
func TestDeleteDraftsByStudentIDRemovesImages(t *testing.T) {
	dao, mock := newActDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `activity_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3001))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?\\)").
		WithArgs("activity_draft", int64(3001)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `activity_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := dao.DB().Transaction(func(tx *gorm.DB) error {
		return dao.DeleteDraftsByStudentID(context.Background(), tx, "S20250001")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindActByOwnerIDAll_NoAuditStatusFilter 本人"全部活动"列表不得按 is_checking 过滤，
// 否则审核中(pending_*)与被拒(reject)的活动对本人不可见（前端 review 页拿不到被拒活动）。
// 与 FindActByOwnerID 的区别正是去掉 SetEffect；本测试断言 SQL 不含 is_checking。
func TestFindActByOwnerIDAll_NoAuditStatusFilter(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(_, actualSQL string) error {
			if strings.Contains(actualSQL, "is_checking") {
				return fmt.Errorf("own-all list must not filter by is_checking, got: %s", actualSQL)
			}
			return nil
		},
	)))
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
	// effect=slow 时 SetEffect 会加 is_checking='pass'；FindActByOwnerIDAll 必须不套它，
	// 否则 matcher 会因 SQL 含 is_checking 报错——这正是本测试要守住的点。
	dao := &ActDao{db: gdb, l: logger.NewLoggerSet().Activity, effect: "slow"}

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := dao.FindActByOwnerIDAll(context.Background(), "S20250001", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 0 || len(got.Acts) != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
