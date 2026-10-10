package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newPostServiceForTest(t *testing.T) (*PostService, *miniredis.Miniredis, sqlmock.Sqlmock) {
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
	postRepo := repo.NewPostRepo(dao.NewPostDao(gdb, &config.Conf{}, ls), cache.NewCache(rdb), ls)
	interRepo := repo.NewInteractionRepo(dao.NewInteractionDao(gdb, ls), nil, nil, nil, cache.NewLikeFavoriteRedis(rdb))
	return &PostService{pdh: postRepo, id: interRepo, l: ls.Post}, mr, mock
}

func seedPostInteractionKeys(t *testing.T, mr *miniredis.Miniredis, postID int64) []string {
	t.Helper()
	keys := []string{
		cache.LikeSetKey(cache.SubjectPost, postID),
		cache.LikeCountKey(cache.SubjectPost, postID),
		cache.CollectSetKey(cache.SubjectPost, postID),
		cache.CollectCountKey(cache.SubjectPost, postID),
	}
	for _, k := range keys {
		if err := mr.Set(k, "1"); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// TestDeletePostClearsInteractionCacheOnHit 删除命中本人帖子后，须清掉该帖在 Redis 的点赞/收藏 key，
// 否则永久残留并可能被复用的 id 继承。
func TestDeletePostClearsInteractionCacheOnHit(t *testing.T) {
	ctx := context.Background()
	svc, mr, mock := newPostServiceForTest(t)
	keys := seedPostInteractionKeys(t, mr, 1001)

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

	if err := svc.DeletePost(ctx, 1001, "S1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range keys {
		if mr.Exists(k) {
			t.Errorf("key %s should be cleared after a successful delete", k)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeletePostKeepsInteractionCacheWhenNotOwned 越权/不存在（删除未命中）时不得触碰 Redis，
// 否则删他人帖子会顺带清空对方帖子的互动缓存。
func TestDeletePostKeepsInteractionCacheWhenNotOwned(t *testing.T) {
	ctx := context.Background()
	svc, mr, mock := newPostServiceForTest(t)
	keys := seedPostInteractionKeys(t, mr, 1001)
	commentKeys := []string{
		cache.LikeSetKey(cache.SubjectComment, 7001),
		cache.LikeCountKey(cache.SubjectComment, 7001),
	}
	for _, k := range commentKeys {
		if err := mr.Set(k, "1"); err != nil {
			t.Fatal(err)
		}
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S-attacker").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := svc.DeletePost(ctx, 1001, "S-attacker"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range append(keys, commentKeys...) {
		if !mr.Exists(k) {
			t.Errorf("key %s must be left intact when delete did not hit", k)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeletePostClearsCommentInteractionCacheOnHit 级联删除的评论，其 Redis 点赞 key 也须清理，
// 避免与帖子 key 同类的永久残留。
func TestDeletePostClearsCommentInteractionCacheOnHit(t *testing.T) {
	ctx := context.Background()
	svc, mr, mock := newPostServiceForTest(t)
	postKeys := seedPostInteractionKeys(t, mr, 1001)
	commentKeys := []string{
		cache.LikeSetKey(cache.SubjectComment, 7001),
		cache.LikeCountKey(cache.SubjectComment, 7001),
		cache.LikeSetKey(cache.SubjectComment, 7002),
		cache.LikeCountKey(cache.SubjectComment, 7002),
	}
	for _, k := range commentKeys {
		if err := mr.Set(k, "1"); err != nil {
			t.Fatal(err)
		}
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `id` FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7001).AddRow(7002))
	mock.ExpectExec("DELETE FROM `user_comment_interaction` WHERE comment_id IN \\(\\?,\\?\\)").
		WithArgs(int64(7001), int64(7002)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `user_post_interaction` WHERE post_id = \\?").
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?\\)").
		WithArgs("post", int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := svc.DeletePost(ctx, 1001, "S1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range commentKeys {
		if mr.Exists(k) {
			t.Errorf("comment key %s should be cleared after a cascaded delete", k)
		}
	}
	for _, k := range postKeys {
		if mr.Exists(k) {
			t.Errorf("post key %s should be cleared", k)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFindPostByIdHidesPrivateFromOthers 私密帖对非作者按"帖子不存在"返回：查询带可见性过滤，
// 命中不到行时抛出 ErrPostNotFound，而非泄露其存在。
func TestFindPostByIdHidesPrivateFromOthers(t *testing.T) {
	ctx := context.Background()
	svc, _, mock := newPostServiceForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `post` WHERE id = \\? AND \\(visibility = \\? OR student_id = \\?\\) ORDER BY `post`\\.`id` LIMIT \\?").
		WithArgs(int64(1001), "public", "S_stranger", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "title", "introduce", "visibility", "is_checking"}))

	_, err := svc.FindPostById(ctx, 1001, "S_stranger")
	if !errors.Is(err, errs.ErrPostNotFound) {
		t.Fatalf("expected ErrPostNotFound for invisible post, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSetVisibilityNotOwnedReturnsNotFound 非作者/帖子不存在时按"帖子不存在"返回，且不改任何行。
func TestSetVisibilityNotOwnedReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	svc, _, mock := newPostServiceForTest(t)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND student_id = \\?").
		WithArgs(int64(1001), "S-attacker").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := svc.SetVisibility(ctx, 1001, "S-attacker", "private")
	if !errors.Is(err, errs.ErrPostNotFound) {
		t.Fatalf("expected ErrPostNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSetVisibilityOwnedSucceeds 作者改自己帖子可见性成功。
func TestSetVisibilityOwnedSucceeds(t *testing.T) {
	ctx := context.Background()
	svc, _, mock := newPostServiceForTest(t)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND student_id = \\?").
		WithArgs(int64(1001), "S1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE `post` SET `visibility`=\\? WHERE id = \\? AND student_id = \\?").
		WithArgs("private", int64(1001), "S1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.SetVisibility(ctx, 1001, "S1", "private"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
