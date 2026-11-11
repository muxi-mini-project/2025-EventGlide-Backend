package dao

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newActGormForTest(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
	return gdb, mock
}

func newActDaoForTest(t *testing.T) (*ActDao, sqlmock.Sqlmock) {
	t.Helper()

	gdb, mock := newActGormForTest(t)
	return &ActDao{db: gdb, l: logger.NewLoggerSet().Activity}, mock
}

// newActDaoViaConfForTest 走 NewActDao 构造，用于覆盖 effect 经 EffectMode 归一化的路径。
func newActDaoViaConfForTest(t *testing.T, effect string) (*ActDao, sqlmock.Sqlmock) {
	t.Helper()

	gdb, mock := newActGormForTest(t)
	cfg := &config.Conf{}
	cfg.Auditor.Effect = effect
	return NewActDao(gdb, cfg, logger.NewLoggerSet()), mock
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

// TestFindPendingAuditorActivitiesSkipsExpired 已过期活动不再送审：它们在所有列表都不可见，
// 送进平台只会占用审核队列。
func TestFindPendingAuditorActivitiesSkipsExpired(t *testing.T) {
	dao, mock := newActDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `activity` WHERE is_checking = 'pending_auditor' AND end_time > \\?").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	acts, err := dao.FindPendingAuditorActivities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(acts) != 0 {
		t.Fatalf("expected no activities, got %+v", acts)
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

// TestListAllActsDefaultsToSlowEffect 未配置 effect 时公开列表只放已通过的活动（fail-closed）：
// 漏配 effect 不能把审核中/被驳回的活动直接暴露在列表里。走 NewActDao 构造，覆盖 EffectMode 归一化。
func TestListAllActsDefaultsToSlowEffect(t *testing.T) {
	dao, mock := newActDaoViaConfForTest(t, "")

	mock.ExpectQuery("SELECT \\* FROM `activity` WHERE end_time > \\? AND is_checking = \\? ORDER BY start_time ASC LIMIT \\?").
		WithArgs(sqlmock.AnyArg(), "pass", 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `activity` WHERE end_time > \\? AND is_checking = \\?").
		WithArgs(sqlmock.AnyArg(), "pass").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	if _, err := dao.ListAllActs(context.Background(), 1, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListAllActsFastEffectOnlyBlocksRejected fast 先发后审：列表只挡被驳回的活动，
// 审核中与已通过都放行。同样走 NewActDao 构造，确保配置值被归一化后生效。
func TestListAllActsFastEffectOnlyBlocksRejected(t *testing.T) {
	dao, mock := newActDaoViaConfForTest(t, config.EffectFast)

	mock.ExpectQuery("SELECT \\* FROM `activity` WHERE end_time > \\? AND is_checking != \\? ORDER BY start_time ASC LIMIT \\?").
		WithArgs(sqlmock.AnyArg(), "reject", 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `activity` WHERE end_time > \\? AND is_checking !=").
		WithArgs(sqlmock.AnyArg(), "reject").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	if _, err := dao.ListAllActs(context.Background(), 1, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGetCheckingExcludesExpiredActivities 审核中列表不返回已过期活动：
// 过期活动不会再送审、也不会有结论，继续以"审核中"出现在 /user/checking 里是错的。
func TestGetCheckingExcludesExpiredActivities(t *testing.T) {
	dao, mock := newActDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `activity` WHERE \\(student_id = \\? AND is_checking IN \\(\\?,\\?\\)\\) AND end_time > \\?").
		WithArgs("S20250001", "pending_signers", "pending_auditor", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if _, err := dao.GetChecking(context.Background(), "S20250001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
