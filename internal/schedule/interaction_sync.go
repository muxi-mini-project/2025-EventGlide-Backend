package schedule

import (
	"context"
	"time"

	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
)

// InteractionSyncTask 互动数据定时同步任务
// 负责热点数据预热和对账修正
type InteractionSyncTask struct {
	lfr *cache.LikeFavoriteRedis
	dao *dao.InteractionDao
	l   *zap.Logger
}

// NewInteractionSyncTask 创建互动数据同步任务
func NewInteractionSyncTask(lfr *cache.LikeFavoriteRedis, dao *dao.InteractionDao, l *zap.Logger) *InteractionSyncTask {
	return &InteractionSyncTask{
		lfr: lfr,
		dao: dao,
		l:   l,
	}
}

// Start 启动定时任务
func (t *InteractionSyncTask) Start(ctx context.Context) {
	// 每5分钟预热热点数据
	go t.hotWarmupLoop(ctx)

	// 每小时对账
	go t.reconcileLoop(ctx)
}

// hotWarmupLoop 热点数据预热循环
func (t *InteractionSyncTask) hotWarmupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// 启动时立即执行一次
	t.warmupHotActivities(ctx)
	t.warmupHotPosts(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.warmupHotActivities(ctx)
			t.warmupHotPosts(ctx)
		}
	}
}

// warmupHotActivities 预热热门活动的点赞/收藏数据
// 从数据库查询最近有互动操作的活动进行预热
func (t *InteractionSyncTask) warmupHotActivities(ctx context.Context) {
	// 查询最近7天有点赞/收藏记录的活动ID
	var activityIds []int64
	err := t.dao.GetRecentActivityIdsWithInteractions(ctx, 100, func(db *model.Activity) bool {
		return db.LikeNum > 0 || db.CollectNum > 0
	}, &activityIds)
	if err != nil {
		t.l.Error("warmupHotActivities failed", zap.Error(err))
		return
	}

	for _, aid := range activityIds {
		// 预热点赞数
		likeCount, err := t.dao.CountAllLikesFromDB(ctx, "activity", aid)
		if err == nil {
			if setErr := t.lfr.SetLikeCount(ctx, cache.SubjectActivity, aid, likeCount); setErr != nil {
				t.l.Warn("SetLikeCount failed", zap.Error(setErr), zap.Int64("activityId", aid))
			}
		}

		// 预热收藏数
		collectCount, err := t.dao.CountAllCollectsFromDB(ctx, "activity", aid)
		if err == nil {
			if setErr := t.lfr.SetCollectCount(ctx, cache.SubjectActivity, aid, collectCount); setErr != nil {
				t.l.Warn("SetCollectCount failed", zap.Error(setErr), zap.Int64("activityId", aid))
			}
		}
	}

	t.l.Info("warmupHotActivities completed", zap.Int("count", len(activityIds)))
}

// warmupHotPosts 预热热门帖子的点赞/收藏数据
func (t *InteractionSyncTask) warmupHotPosts(ctx context.Context) {
	// 查询最近有互动操作的帖子ID
	var postIds []int64
	err := t.dao.GetRecentPostIdsWithInteractions(ctx, 100, func(db *model.Post) bool {
		return db.LikeNum > 0 || db.CollectNum > 0
	}, &postIds)
	if err != nil {
		t.l.Error("warmupHotPosts failed", zap.Error(err))
		return
	}

	for _, pid := range postIds {
		// 预热点赞数
		likeCount, err := t.dao.CountAllLikesFromDB(ctx, "post", pid)
		if err == nil {
			if setErr := t.lfr.SetLikeCount(ctx, cache.SubjectPost, pid, likeCount); setErr != nil {
				t.l.Warn("SetLikeCount failed", zap.Error(setErr), zap.Int64("postId", pid))
			}
		}

		// 预热收藏数
		collectCount, err := t.dao.CountAllCollectsFromDB(ctx, "post", pid)
		if err == nil {
			if setErr := t.lfr.SetCollectCount(ctx, cache.SubjectPost, pid, collectCount); setErr != nil {
				t.l.Warn("SetCollectCount failed", zap.Error(setErr), zap.Int64("postId", pid))
			}
		}
	}

	t.l.Info("warmupHotPosts completed", zap.Int("count", len(postIds)))
}

// reconcileLoop 对账循环
func (t *InteractionSyncTask) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.reconcileAll(ctx)
		}
	}
}

// reconcileAll 全量对账
func (t *InteractionSyncTask) reconcileAll(ctx context.Context) {
	t.l.Info("reconcileAll started")

	// 获取最近有互动记录的活动进行对账
	var activityIds []int64
	if err := t.dao.GetRecentActivityIdsWithInteractions(ctx, 500, nil, &activityIds); err != nil {
		t.l.Error("GetRecentActivityIdsWithInteractions failed", zap.Error(err))
	}

	for _, aid := range activityIds {
		t.reconcileActivity(ctx, aid)
	}

	// 获取最近有互动记录的帖子进行对账
	var postIds []int64
	if err := t.dao.GetRecentPostIdsWithInteractions(ctx, 500, nil, &postIds); err != nil {
		t.l.Error("GetRecentPostIdsWithInteractions failed", zap.Error(err))
	}

	for _, pid := range postIds {
		t.reconcilePost(ctx, pid)
	}

	t.l.Info("reconcileAll completed")
}

// reconcileActivity 对账单个活动
func (t *InteractionSyncTask) reconcileActivity(ctx context.Context, activityId int64) {
	// 获取 Redis 计数
	redisLikeCount, exists, err := t.lfr.GetLikeCount(ctx, cache.SubjectActivity, activityId)
	if err != nil {
		t.l.Warn("GetLikeCount from Redis failed", zap.Error(err), zap.Int64("activityId", activityId))
		return
	}
	if !exists {
		return
	}

	redisCollectCount, exists, err := t.lfr.GetCollectCount(ctx, cache.SubjectActivity, activityId)
	if err != nil {
		t.l.Warn("GetCollectCount from Redis failed", zap.Error(err), zap.Int64("activityId", activityId))
		return
	}
	if !exists {
		return
	}

	// 获取 MySQL 计数
	dbLikeCount, err := t.dao.CountAllLikesFromDB(ctx, "activity", activityId)
	if err != nil {
		t.l.Warn("CountAllLikesFromDB failed", zap.Error(err), zap.Int64("activityId", activityId))
		return
	}

	dbCollectCount, err := t.dao.CountAllCollectsFromDB(ctx, "activity", activityId)
	if err != nil {
		t.l.Warn("CountAllCollectsFromDB failed", zap.Error(err), zap.Int64("activityId", activityId))
		return
	}

	// 不一致则修正 MySQL
	if redisLikeCount != dbLikeCount {
		t.l.Info("Like count mismatch, fixing",
			zap.Int64("activityId", activityId),
			zap.Int64("redisCount", redisLikeCount),
			zap.Int64("dbCount", dbLikeCount),
		)
		if err := t.dao.FixActivityLikeNum(ctx, activityId, redisLikeCount); err != nil {
			t.l.Error("FixActivityLikeNum failed", zap.Error(err), zap.Int64("activityId", activityId))
		}
	}

	if redisCollectCount != dbCollectCount {
		t.l.Info("Collect count mismatch, fixing",
			zap.Int64("activityId", activityId),
			zap.Int64("redisCount", redisCollectCount),
			zap.Int64("dbCount", dbCollectCount),
		)
		if err = t.dao.FixActivityCollectNum(ctx, activityId, redisCollectCount); err != nil {
			t.l.Error("FixActivityCollectNum failed", zap.Error(err), zap.Int64("activityId", activityId))
		}
	}
}

// reconcilePost 对账单个帖子
func (t *InteractionSyncTask) reconcilePost(ctx context.Context, postId int64) {
	// 获取 Redis 计数
	redisLikeCount, exists, err := t.lfr.GetLikeCount(ctx, cache.SubjectPost, postId)
	if err != nil {
		t.l.Warn("GetLikeCount from Redis failed", zap.Error(err), zap.Int64("postId", postId))
		return
	}
	if !exists {
		return
	}

	redisCollectCount, exists, err := t.lfr.GetCollectCount(ctx, cache.SubjectPost, postId)
	if err != nil {
		t.l.Warn("GetCollectCount from Redis failed", zap.Error(err), zap.Int64("postId", postId))
		return
	}
	if !exists {
		return
	}

	// 获取 MySQL 计数
	dbLikeCount, err := t.dao.CountAllLikesFromDB(ctx, "post", postId)
	if err != nil {
		t.l.Warn("CountAllLikesFromDB failed", zap.Error(err), zap.Int64("postId", postId))
		return
	}

	dbCollectCount, err := t.dao.CountAllCollectsFromDB(ctx, "post", postId)
	if err != nil {
		t.l.Warn("CountAllCollectsFromDB failed", zap.Error(err), zap.Int64("postId", postId))
		return
	}

	// 不一致则修正 MySQL
	if redisLikeCount != dbLikeCount {
		t.l.Info("Like count mismatch, fixing",
			zap.Int64("postId", postId),
			zap.Int64("redisCount", redisLikeCount),
			zap.Int64("dbCount", dbLikeCount),
		)
		if err = t.dao.FixPostLikeNum(ctx, postId, redisLikeCount); err != nil {
			t.l.Error("FixPostLikeNum failed", zap.Error(err), zap.Int64("postId", postId))
		}
	}

	if redisCollectCount != dbCollectCount {
		t.l.Info("Collect count mismatch, fixing",
			zap.Int64("postId", postId),
			zap.Int64("redisCount", redisCollectCount),
			zap.Int64("dbCount", dbCollectCount),
		)
		if err := t.dao.FixPostCollectNum(ctx, postId, redisCollectCount); err != nil {
			t.l.Error("FixPostCollectNum failed", zap.Error(err), zap.Int64("postId", postId))
		}
	}
}
