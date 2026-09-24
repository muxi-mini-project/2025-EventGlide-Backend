package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newPostDaoForTest(t *testing.T) (*PostDao, sqlmock.Sqlmock) {
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

	return &PostDao{db: gdb, l: logger.NewLoggerSet().Post}, mock
}

// TestFindPendingAuditorPostsSQL 锁定待送审帖子的筛选条件（is_checking='checking'），
// 并确认预加载图片——worker 依赖完整行（含图片）重建送审请求。
func TestFindPendingAuditorPostsSQL(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `post` WHERE is_checking = 'checking'").
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "title", "introduce", "is_checking"}).
			AddRow(1001, "S20250001", "招新", "正文", "checking"))
	mock.ExpectQuery("SELECT \\* FROM `image` WHERE `image`.`owner_id` = \\?").
		WithArgs(1001).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "owner_type", "url"}))

	posts, err := dao.FindPendingAuditorPosts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 1 || posts[0].Id != 1001 {
		t.Fatalf("expected the single checking post, got %+v", posts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
