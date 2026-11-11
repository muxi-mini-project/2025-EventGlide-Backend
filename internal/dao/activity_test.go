package dao

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newActGormWithMatcherForTest(t *testing.T, matcher sqlmock.QueryMatcher) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
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

func newActGormForTest(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	return newActGormWithMatcherForTest(t, sqlmock.QueryMatcherRegexp)
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

// TestFindActListForStudent_NoAuditStatusFilter 本人活动列表（默认 active/creator）不得按 is_checking 过滤，
// 否则审核中(pending_*)与被拒(reject)的活动对本人不可见（前端 review 页拿不到被拒活动）。
// 与 FindActByOwnerID 的区别正是去掉 SetEffect；本测试断言 SQL 不含 is_checking。
func TestFindActListForStudent_NoAuditStatusFilter(t *testing.T) {
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
	// effect=slow 时 SetEffect 会加 is_checking='pass'；FindActListForStudent 必须不套它，
	// 否则 matcher 会因 SQL 含 is_checking 报错——这正是本测试要守住的点。
	dao := &ActDao{db: gdb, l: logger.NewLoggerSet().Activity, effect: "slow"}

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := dao.FindActListForStudent(context.Background(), "S20250001", req.ActScopeActive, req.ActRoleCreator, 1, 10)
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

// TestFindActListForStudent_DefaultScopeRole 未传 scope/role（空值）时按 active/creator 处理：
// 只查本人创建且未结束的活动，不按审核状态过滤，也不走 approvement 子查询。
func TestFindActListForStudent_DefaultScopeRole(t *testing.T) {
	const sid = "S20250001"

	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		switch expected {
		case "list":
			if !strings.HasPrefix(actual, "SELECT * FROM `activity`") {
				return fmt.Errorf("unexpected list SQL: %s", actual)
			}
			if !strings.Contains(actual, "ORDER BY start_time ASC, id ASC") {
				return fmt.Errorf("list SQL missing stable order: %s", actual)
			}
		case "count":
			if !strings.Contains(actual, "count(*)") {
				return fmt.Errorf("unexpected count SQL: %s", actual)
			}
		}
		for _, f := range []string{"student_id = ?", "end_time > ?"} {
			if !strings.Contains(actual, f) {
				return fmt.Errorf("%s SQL missing condition %q: %s", expected, f, actual)
			}
		}
		for _, f := range []string{"is_checking", "approvement"} {
			if strings.Contains(actual, f) {
				return fmt.Errorf("%s SQL must not contain %q: %s", expected, f, actual)
			}
		}
		return nil
	})

	gdb, mock := newActGormWithMatcherForTest(t, matcher)
	dao := &ActDao{db: gdb, l: logger.NewLoggerSet().Activity}

	mock.ExpectQuery("list").WithArgs(sid, sqlmock.AnyArg(), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("count").WithArgs(sid, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	if _, err := dao.FindActListForStudent(context.Background(), sid, "", "", 1, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindActListForStudentSQL scope 与 role 的组合必须体现在 WHERE 中：
// scope 控制时间窗口、已结束活动的 pass 过滤与排序；role 控制创建者/签署人（approvement 子查询）。
// 列表与 count 必须命中同一套条件（同一组 fragments 分别断言两条 SQL）。
func TestFindActListForStudentSQL(t *testing.T) {
	const sid = "S20250001"

	cases := []struct {
		name      string
		scope     string
		role      string
		where     []string
		order     string
		listArgs  []driver.Value
		countArgs []driver.Value
		absent    []string
	}{
		{
			name:      "active/creator 默认仅进行中且不按审核状态过滤",
			scope:     req.ActScopeActive,
			role:      req.ActRoleCreator,
			where:     []string{"student_id = ?", "end_time > ?"},
			order:     "ORDER BY start_time ASC, id ASC",
			listArgs:  []driver.Value{sid, sqlmock.AnyArg(), 10},
			countArgs: []driver.Value{sid, sqlmock.AnyArg()},
			absent:    []string{"is_checking"},
		},
		{
			name:      "ended/creator 只放已通过并按结束时间倒序",
			scope:     req.ActScopeEnded,
			role:      req.ActRoleCreator,
			where:     []string{"student_id = ?", "end_time <= ?", "is_checking = ?"},
			order:     "ORDER BY end_time DESC, id DESC",
			listArgs:  []driver.Value{sid, sqlmock.AnyArg(), "pass", 10},
			countArgs: []driver.Value{sid, sqlmock.AnyArg(), "pass"},
		},
		{
			name:      "all/creator 进行中或已通过",
			scope:     req.ActScopeAll,
			role:      req.ActRoleCreator,
			where:     []string{"student_id = ?", "end_time > ?", "OR is_checking = ?"},
			order:     "ORDER BY start_time DESC, id DESC",
			listArgs:  []driver.Value{sid, sqlmock.AnyArg(), "pass", 10},
			countArgs: []driver.Value{sid, sqlmock.AnyArg(), "pass"},
		},
		{
			name:      "active/participant 走签署子查询",
			scope:     req.ActScopeActive,
			role:      req.ActRoleParticipant,
			where:     []string{"id IN (SELECT", "FROM `approvement`", "student_id = ?", "end_time > ?"},
			order:     "ORDER BY start_time ASC, id ASC",
			listArgs:  []driver.Value{sid, sqlmock.AnyArg(), 10},
			countArgs: []driver.Value{sid, sqlmock.AnyArg()},
		},
		{
			name:      "ended/participant 子查询叠加 pass 过滤",
			scope:     req.ActScopeEnded,
			role:      req.ActRoleParticipant,
			where:     []string{"id IN (SELECT", "FROM `approvement`", "end_time <= ?", "is_checking = ?"},
			order:     "ORDER BY end_time DESC, id DESC",
			listArgs:  []driver.Value{sid, sqlmock.AnyArg(), "pass", 10},
			countArgs: []driver.Value{sid, sqlmock.AnyArg(), "pass"},
		},
		{
			name:      "all/all 创建者与签署人并集",
			scope:     req.ActScopeAll,
			role:      req.ActRoleAll,
			where:     []string{"student_id = ?", "OR id IN (SELECT", "FROM `approvement`", "end_time > ?", "OR is_checking = ?"},
			order:     "ORDER BY start_time DESC, id DESC",
			listArgs:  []driver.Value{sid, sid, sqlmock.AnyArg(), "pass", 10},
			countArgs: []driver.Value{sid, sid, sqlmock.AnyArg(), "pass"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
				if expected == "list" {
					if !strings.HasPrefix(actual, "SELECT * FROM `activity`") {
						return fmt.Errorf("unexpected list SQL: %s", actual)
					}
					if !strings.Contains(actual, tc.order) {
						return fmt.Errorf("list SQL missing order %q: %s", tc.order, actual)
					}
				} else {
					if !strings.Contains(actual, "count(*)") {
						return fmt.Errorf("unexpected count SQL: %s", actual)
					}
				}
				for _, f := range tc.where {
					if !strings.Contains(actual, f) {
						return fmt.Errorf("%s SQL missing condition %q: %s", expected, f, actual)
					}
				}
				for _, f := range tc.absent {
					if strings.Contains(actual, f) {
						return fmt.Errorf("%s SQL must not contain %q: %s", expected, f, actual)
					}
				}
				return nil
			})

			gdb, mock := newActGormWithMatcherForTest(t, matcher)
			dao := &ActDao{db: gdb, l: logger.NewLoggerSet().Activity}

			mock.ExpectQuery("list").WithArgs(tc.listArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery("count").WithArgs(tc.countArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			if _, err := dao.FindActListForStudent(context.Background(), sid, tc.scope, tc.role, 1, 10); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
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
