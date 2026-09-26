package repo

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newPostRepoForTest(t *testing.T) (*PostRepo, *miniredis.Miniredis, sqlmock.Sqlmock) {
	t.Helper()

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

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

	ls := logger.NewLoggerSet()
	pr := NewPostRepo(dao.NewPostDao(gdb, &config.Conf{}, ls), cache.NewCache(rdb), ls)
	return pr, mr, mock
}

// TestDeletePostInvalidateFailureIsBestEffort 缓存失效（post:id key）失败是尽力而为，
// 不得让已提交的删除返回错误——否则 service 会提前返回、跳过后续互动缓存清理。
func TestDeletePostInvalidateFailureIsBestEffort(t *testing.T) {
	ctx := context.Background()
	pr, mr, mock := newPostRepoForTest(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `id` FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("DELETE FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `user_post_interaction` WHERE post_id = \\?").
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?\\)").
		WithArgs("post", int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	// 让缓存失效失败：Redis 报错。
	mr.SetError("redis down")

	deleted, _, err := pr.DeletePost(ctx, &model.Post{Id: 1001, StudentID: "S1"})
	if err != nil {
		t.Fatalf("invalidate failure must not surface as a delete error, got %v", err)
	}
	if !deleted {
		t.Fatalf("expected deleted=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
