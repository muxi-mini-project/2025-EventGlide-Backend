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

// TestFindPendingAuditorPostsSQL 锁定待送审帖子的筛选契约：
//   - 仍为 checking；
//   - 排除已有已推送审核表单的行（帖子回调前一直 checking，否则每 5s 会把已送审项重扫一遍）；
//   - 预加载图片——worker 依赖完整行（含图片）重建送审请求。
func TestFindPendingAuditorPostsSQL(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `post` WHERE is_checking = \\? AND \\(NOT EXISTS \\(SELECT 1 FROM auditor_form af WHERE af.activity_id = post.id AND af.subject = \\? AND af.pushed_at IS NOT NULL\\)\\)").
		WithArgs("checking", "post").
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
