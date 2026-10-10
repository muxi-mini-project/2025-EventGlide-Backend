package repo

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// newInteractionRepoForTest 构造一个真实 Redis + sqlmock DB 的 InteractionRepo，
// 用于验证 Redis 异常时的批量 SQL 降级路径。
func newInteractionRepoForTest(t *testing.T) (*InteractionRepo, *miniredis.Miniredis, sqlmock.Sqlmock) {
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
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	idao := dao.NewInteractionDao(gdb, logger.NewLoggerSet())
	lfr := cache.NewLikeFavoriteRedis(rdb)
	return NewInteractionRepo(idao, nil, nil, nil, lfr), mr, mock
}

func TestGetUserPostInteractionMapsFallback(t *testing.T) {
	ctx := context.Background()
	r, mr, mock := newInteractionRepoForTest(t)

	postIds := []int64{1, 2, 3}
	userId := int64(9)

	// 模拟 Redis 宕机
	mr.SetError("simulate redis down")

	// 降级为两条批量 SQL：like 与 collect
	rows := sqlmock.NewRows([]string{"post_id"}).
		AddRow(1).
		AddRow(3)
	mock.ExpectQuery("SELECT `post_id` FROM `user_post_interaction` WHERE user_id = \\? AND post_id IN \\(\\?,\\?,\\?\\) AND type = \\?").
		WithArgs(userId, postIds[0], postIds[1], postIds[2], "like").
		WillReturnRows(rows)
	rows2 := sqlmock.NewRows([]string{"post_id"}).AddRow(2)
	mock.ExpectQuery("SELECT `post_id` FROM `user_post_interaction` WHERE user_id = \\? AND post_id IN \\(\\?,\\?,\\?\\) AND type = \\?").
		WithArgs(userId, postIds[0], postIds[1], postIds[2], "collect").
		WillReturnRows(rows2)

	likedMap, collectedMap, err := r.GetUserPostInteractionMaps(ctx, userId, postIds)
	if err != nil {
		t.Fatal(err)
	}

	// 降级路径稠密化语义：每个 id 均有 entry 且默认 false
	wantLiked := map[int64]bool{1: true, 2: false, 3: true}
	wantCollected := map[int64]bool{1: false, 2: true, 3: false}
	for id, want := range wantLiked {
		if likedMap[id] != want {
			t.Errorf("likedMap[%d] = %v, want %v", id, likedMap[id], want)
		}
	}
	for id, want := range wantCollected {
		if collectedMap[id] != want {
			t.Errorf("collectedMap[%d] = %v, want %v", id, collectedMap[id], want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetUserLikedCommentMapFallback(t *testing.T) {
	ctx := context.Background()
	r, mr, mock := newInteractionRepoForTest(t)

	commentIds := []int64{11, 22}
	userId := int64(5)

	mr.SetError("simulate redis down")

	rows := sqlmock.NewRows([]string{"comment_id"}).AddRow(22)
	mock.ExpectQuery("SELECT `comment_id` FROM `user_comment_interaction` WHERE user_id = \\? AND comment_id IN \\(\\?,\\?\\) AND type = \\?").
		WithArgs(userId, commentIds[0], commentIds[1], "like").
		WillReturnRows(rows)

	got, err := r.GetUserLikedCommentMap(ctx, userId, commentIds)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]bool{11: false, 22: true}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("got[%d] = %v, want %v", id, got[id], w)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetUserPostInteractionMapsEmpty(t *testing.T) {
	ctx := context.Background()
	r, _, _ := newInteractionRepoForTest(t)

	likedMap, collectedMap, err := r.GetUserPostInteractionMaps(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(likedMap) != 0 || len(collectedMap) != 0 {
		t.Errorf("expect empty maps, got liked=%d collected=%d", len(likedMap), len(collectedMap))
	}

	got, err := r.GetUserLikedCommentMap(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expect empty map, got %d", len(got))
	}
}

// TestGetUserLikedPostIdsFiltersByViewerVisibility 点赞列表的 Total 与当页 ids 必须在同一 SQL 内
// 按 viewer 可见性过滤（join post，公开或本人帖，且排除历史 reject 帖），避免过滤后置导致 Total 偏大、分页不足。
func TestGetUserLikedPostIdsFiltersByViewerVisibility(t *testing.T) {
	ctx := context.Background()
	r, _, mock := newInteractionRepoForTest(t)
	const viewer = "S_owner"
	userId := int64(9)

	joinSQL := "FROM user_post_interaction AS upi JOIN post p ON p.id = upi.post_id WHERE"
	mock.ExpectQuery("SELECT count\\(\\*\\) "+joinSQL).
		WithArgs(userId, "like", model.PostVisibilityPublic, viewer, "reject").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* "+joinSQL).
		WithArgs(userId, "like", model.PostVisibilityPublic, viewer, "reject", 10).
		WillReturnRows(sqlmock.NewRows([]string{"post_id"}).AddRow(1001))

	got, err := r.GetUserLikedPostIds(ctx, userId, viewer, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Ids) != 1 || got.Ids[0] != 1001 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestGetUserCollectedPostIdsFiltersByViewerVisibility 收藏列表与点赞列表同一过滤口径。
func TestGetUserCollectedPostIdsFiltersByViewerVisibility(t *testing.T) {
	ctx := context.Background()
	r, _, mock := newInteractionRepoForTest(t)
	const viewer = "S_owner"
	userId := int64(9)

	joinSQL := "FROM user_post_interaction AS upi JOIN post p ON p.id = upi.post_id WHERE"
	mock.ExpectQuery("SELECT count\\(\\*\\) "+joinSQL).
		WithArgs(userId, "collect", model.PostVisibilityPublic, viewer, "reject").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* "+joinSQL).
		WithArgs(userId, "collect", model.PostVisibilityPublic, viewer, "reject", 10).
		WillReturnRows(sqlmock.NewRows([]string{"post_id"}).AddRow(1002))

	got, err := r.GetUserCollectedPostIds(ctx, userId, viewer, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Ids) != 1 || got.Ids[0] != 1002 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
