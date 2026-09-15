package dao

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newFeedDaoForTest(t *testing.T) (*FeedDao, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	return NewFeedDao(gdb, logger.NewLoggerSet()), mock
}

// TestFeedListQueriesAreOrderedAscending 锁定 feed 列表查询的排序契约：
// 前端把 feed 列表当升序输入做归并再倒序展示，排序键与方向都需保持稳定。
func TestFeedListQueriesAreOrderedAscending(t *testing.T) {
	ctx := context.Background()
	const sid = "S20250001"
	const feedOrder = " ORDER BY created_at ASC, id ASC$"

	cases := []struct {
		name  string
		query string
		args  []driver.Value
		run   func(*FeedDao) error
	}{
		{
			name:  "like",
			query: "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\?" + feedOrder,
			args:  []driver.Value{sid, "like", sid},
			run: func(fd *FeedDao) error {
				_, err := fd.GetLikeFeed(ctx, sid)
				return err
			},
		},
		{
			name:  "collect",
			query: "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\?" + feedOrder,
			args:  []driver.Value{sid, "collect", sid},
			run: func(fd *FeedDao) error {
				_, err := fd.GetCollectFeed(ctx, sid)
				return err
			},
		},
		{
			name:  "comment",
			query: "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\?" + feedOrder,
			args:  []driver.Value{sid, "comment", sid},
			run: func(fd *FeedDao) error {
				_, err := fd.GetCommentFeed(ctx, sid)
				return err
			},
		},
		{
			name:  "at",
			query: "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\?" + feedOrder,
			args:  []driver.Value{sid, "at", sid},
			run: func(fd *FeedDao) error {
				_, err := fd.GetAtFeed(ctx, sid)
				return err
			},
		},
		{
			name:  "auditor",
			query: "SELECT \\* FROM `approvement` WHERE stance = \\? and student_id = \\? ORDER BY created_at ASC, id ASC$",
			args:  []driver.Value{"pending", sid},
			run: func(fd *FeedDao) error {
				_, err := fd.GetAuditorFeed(ctx, sid)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fd, mock := newFeedDaoForTest(t)

			mock.ExpectQuery(tc.query).WithArgs(tc.args...).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))

			if err := tc.run(fd); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}
