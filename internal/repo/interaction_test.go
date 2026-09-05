package repo

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
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