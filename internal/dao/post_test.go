package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/internal/model"
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

// TestDeletePostRemovesImages 删除帖子须在同一事务内一并清理其图片。
func TestDeletePostRemovesImages(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S20250001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?\\)").
		WithArgs("post", int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := dao.DeletePost(context.Background(), &model.Post{Id: 1001, StudentID: "S20250001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeletePostNotOwnedKeepsImages 帖子不属该学生（删除未命中）时不应触碰图片，
// 否则会越权删除他人帖子的图片。
func TestDeletePostNotOwnedKeepsImages(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S-attacker").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := dao.DeletePost(context.Background(), &model.Post{Id: 1001, StudentID: "S-attacker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeleteDraftByStudentRemovesImages 删除草稿须清理其 post_draft 图片（草稿保存先删后建，
// 每次保存都重插 image 行）。
func TestDeleteDraftByStudentRemovesImages(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `post_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2001).AddRow(2002))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?,\\?\\)").
		WithArgs("post_draft", int64(2001), int64(2002)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM `post_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := dao.DB().Transaction(func(tx *gorm.DB) error {
		return dao.DeleteDraftByStudent(context.Background(), tx, "S20250001")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
