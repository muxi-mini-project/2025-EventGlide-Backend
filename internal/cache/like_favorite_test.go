package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestLikeFavoriteRedis(t *testing.T) (*LikeFavoriteRedis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewLikeFavoriteRedis(rdb), mr
}

func TestMGetInteractionStatus(t *testing.T) {
	ctx := context.Background()
	l, _ := newTestLikeFavoriteRedis(t)

	const userID = int64(42)
	ids := []int64{1, 2, 3, 4}

	// 预置状态：1 点赞，2 收藏，3 点赞+收藏，4 无互动
	if err := l.rdb.SAdd(ctx, LikeSetKey(SubjectPost, 1), userID).Err(); err != nil {
		t.Fatal(err)
	}
	if err := l.rdb.SAdd(ctx, CollectSetKey(SubjectPost, 2), userID).Err(); err != nil {
		t.Fatal(err)
	}
	if err := l.rdb.SAdd(ctx, LikeSetKey(SubjectPost, 3), userID).Err(); err != nil {
		t.Fatal(err)
	}
	if err := l.rdb.SAdd(ctx, CollectSetKey(SubjectPost, 3), userID).Err(); err != nil {
		t.Fatal(err)
	}

	likedMap, collectedMap, err := l.MGetUserInteractionStatus(ctx, SubjectPost, ids, userID)
	if err != nil {
		t.Fatal(err)
	}

	// 与单点查询语义一致：每个 id 必有 entry
	for _, id := range ids {
		if _, ok := likedMap[id]; !ok {
			t.Errorf("likedMap missing entry for id %d", id)
		}
		if _, ok := collectedMap[id]; !ok {
			t.Errorf("collectedMap missing entry for id %d", id)
		}
	}
	wantLiked := map[int64]bool{1: true, 2: false, 3: true, 4: false}
	wantCollected := map[int64]bool{1: false, 2: true, 3: true, 4: false}
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
}

func TestMGetInteractionStatusEmpty(t *testing.T) {
	ctx := context.Background()
	l, _ := newTestLikeFavoriteRedis(t)

	likedMap, collectedMap, err := l.MGetUserInteractionStatus(ctx, SubjectPost, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(likedMap) != 0 || len(collectedMap) != 0 {
		t.Errorf("expect empty maps, got liked=%d collected=%d", len(likedMap), len(collectedMap))
	}

	lm, err := l.MGetLikedStatusesForUser(ctx, SubjectPost, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(lm) != 0 {
		t.Errorf("expect empty map, got %d", len(lm))
	}

	cm, err := l.MGetCollectedStatusesForUser(ctx, SubjectPost, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(cm) != 0 {
		t.Errorf("expect empty map, got %d", len(cm))
	}
}

func TestMGetInteractionStatusRedisError(t *testing.T) {
	ctx := context.Background()
	l, mr := newTestLikeFavoriteRedis(t)
	mr.SetError("simulate redis down")

	if _, _, err := l.MGetUserInteractionStatus(ctx, SubjectPost, []int64{1}, 1); err == nil {
		t.Error("expect error when redis is down")
	}
	if _, err := l.MGetLikedStatusesForUser(ctx, SubjectPost, []int64{1}, 1); err == nil {
		t.Error("expect error when redis is down")
	}
	if _, err := l.MGetCollectedStatusesForUser(ctx, SubjectPost, []int64{1}, 1); err == nil {
		t.Error("expect error when redis is down")
	}
}

func TestMGetLikedStatusesForUser(t *testing.T) {
	ctx := context.Background()
	l, _ := newTestLikeFavoriteRedis(t)

	const userID = int64(7)
	ids := []int64{10, 20, 30}

	if err := l.rdb.SAdd(ctx, LikeSetKey(SubjectComment, 20), userID).Err(); err != nil {
		t.Fatal(err)
	}

	got, err := l.MGetLikedStatusesForUser(ctx, SubjectComment, ids, userID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]bool{10: false, 20: true, 30: false}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("got[%d] = %v, want %v", id, got[id], w)
		}
	}
}
