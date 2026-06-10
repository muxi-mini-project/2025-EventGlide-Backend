package repo

import (
	"context"

	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
)

type InteractionRepo struct {
	dao   *dao.InteractionDao
	users *UserRepo
	acts  *ActivityRepo
	posts *PostRepo
	lfr   *cache.LikeFavoriteRedis // Redis 缓存层
}

func NewInteractionRepo(dao *dao.InteractionDao, users *UserRepo, acts *ActivityRepo, posts *PostRepo, lfr *cache.LikeFavoriteRedis) *InteractionRepo {
	return &InteractionRepo{
		dao:   dao,
		users: users,
		acts:  acts,
		posts: posts,
		lfr:   lfr,
	}
}

// GetUserIDByStudentID 根据学生 ID 获取用户 ID
func (r *InteractionRepo) GetUserIDByStudentID(ctx context.Context, studentID string) (int64, error) {
	user, err := r.users.GetUserInfo(ctx, studentID)
	if err != nil {
		return 0, err
	}
	return int64(user.Id), nil
}

func (r *InteractionRepo) LikeActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.LikeActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) LikePost(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.LikePost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) LikeComment(ctx context.Context, studentID string, targetID int64) error {
	return r.dao.LikeComment(ctx, studentID, targetID)
}

func (r *InteractionRepo) DislikeActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.DislikeActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DislikePost(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.DislikePost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DislikeComment(ctx context.Context, studentID string, targetID int64) error {
	return r.dao.DislikeComment(ctx, studentID, targetID)
}

func (r *InteractionRepo) CommentActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.CommentActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) CommentPost(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.CommentPost(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.posts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) CollectActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.CollectActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) CollectPost(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.CollectPost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DiscollectActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.DiscollectActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DiscollectPost(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.DiscollectPost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) ApproveActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.ApproveActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) RejectActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.RejectActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) InsertApprovement(ctx context.Context, studentID, studentName string, targetID int64) error {
	return r.dao.InsertApprovement(ctx, studentID, studentName, targetID)
}

func (r *InteractionRepo) IsUserLikedActivity(ctx context.Context, userId, activityId int64) bool {
	// 优先查 Redis
	liked, err := r.lfr.IsLiked(ctx, cache.SubjectActivity, activityId, userId)
	if err == nil {
		return liked
	}
	// Redis 异常降级查 MySQL
	return r.dao.IsUserLikedActivity(ctx, userId, activityId)
}

func (r *InteractionRepo) IsUserCollectedActivity(ctx context.Context, userId, activityId int64) bool {
	// 优先查 Redis
	collected, err := r.lfr.IsCollected(ctx, cache.SubjectActivity, activityId, userId)
	if err == nil {
		return collected
	}
	// Redis 异常降级查 MySQL
	return r.dao.IsUserCollectedActivity(ctx, userId, activityId)
}

func (r *InteractionRepo) IsUserLikedPost(ctx context.Context, userId, postId int64) bool {
	// 优先查 Redis
	liked, err := r.lfr.IsLiked(ctx, cache.SubjectPost, postId, userId)
	if err == nil {
		return liked
	}
	// Redis 异常降级查 MySQL
	return r.dao.IsUserLikedPost(ctx, userId, postId)
}

func (r *InteractionRepo) IsUserCollectedPost(ctx context.Context, userId, postId int64) bool {
	// 优先查 Redis
	collected, err := r.lfr.IsCollected(ctx, cache.SubjectPost, postId, userId)
	if err == nil {
		return collected
	}
	// Redis 异常降级查 MySQL
	return r.dao.IsUserCollectedPost(ctx, userId, postId)
}

func (r *InteractionRepo) IsUserLikedComment(ctx context.Context, userId, commentId int64) bool {
	// 优先查 Redis
	liked, err := r.lfr.IsLiked(ctx, cache.SubjectComment, commentId, userId)
	if err == nil {
		return liked
	}
	// Redis 异常降级查 MySQL
	return r.dao.IsUserLikedComment(ctx, userId, commentId)
}

func (r *InteractionRepo) GetUserCollectedActivityIds(ctx context.Context, userId int64, page, limit int) (*model.PaginatedActivityIds, error) {
	return r.dao.GetUserCollectedActivityIds(ctx, userId, page, limit)
}

func (r *InteractionRepo) GetUserLikedActivityIds(ctx context.Context, userId int64, page, limit int) (*model.PaginatedActivityIds, error) {
	return r.dao.GetUserLikedActivityIds(ctx, userId, page, limit)
}

func (r *InteractionRepo) GetUserCollectedPostIds(ctx context.Context, userId int64, page, limit int) (*model.PaginatedPostIds, error) {
	return r.dao.GetUserCollectedPostIds(ctx, userId, page, limit)
}

func (r *InteractionRepo) GetUserLikedPostIds(ctx context.Context, userId int64, page, limit int) (*model.PaginatedPostIds, error) {
	return r.dao.GetUserLikedPostIds(ctx, userId, page, limit)
}

func (r *InteractionRepo) GetUserActivityInteractionStatuses(ctx context.Context, userId int64, activityIds []int64) ([]int64, []int64, error) {
	if len(activityIds) == 0 {
		return []int64{}, []int64{}, nil
	}

	// 批量查询点赞状态
	likedMap := make(map[int64]bool)
	for _, aid := range activityIds {
		liked, err := r.lfr.IsLiked(ctx, cache.SubjectActivity, aid, userId)
		if err != nil {
			return r.dao.GetUserActivityInteractionStatuses(ctx, userId, activityIds)
		}
		if liked {
			likedMap[aid] = true
		}
	}

	// 批量查询收藏状态
	collectedMap := make(map[int64]bool)
	for _, aid := range activityIds {
		collected, err := r.lfr.IsCollected(ctx, cache.SubjectActivity, aid, userId)
		if err != nil {
			return r.dao.GetUserActivityInteractionStatuses(ctx, userId, activityIds)
		}
		if collected {
			collectedMap[aid] = true
		}
	}

	likedIds := make([]int64, 0)
	collectedIds := make([]int64, 0)
	for _, aid := range activityIds {
		if likedMap[aid] {
			likedIds = append(likedIds, aid)
		}
		if collectedMap[aid] {
			collectedIds = append(collectedIds, aid)
		}
	}

	return likedIds, collectedIds, nil
}

func (r *InteractionRepo) GetUserPostInteractionStatuses(ctx context.Context, userId int64, postIds []int64) ([]int64, []int64, error) {
	if len(postIds) == 0 {
		return []int64{}, []int64{}, nil
	}

	// 批量查询点赞状态
	likedMap := make(map[int64]bool)
	for _, pid := range postIds {
		liked, err := r.lfr.IsLiked(ctx, cache.SubjectPost, pid, userId)
		if err != nil {
			return r.dao.GetUserPostInteractionStatuses(ctx, userId, postIds)
		}
		if liked {
			likedMap[pid] = true
		}
	}

	// 批量查询收藏状态
	collectedMap := make(map[int64]bool)
	for _, pid := range postIds {
		collected, err := r.lfr.IsCollected(ctx, cache.SubjectPost, pid, userId)
		if err != nil {
			return r.dao.GetUserPostInteractionStatuses(ctx, userId, postIds)
		}
		if collected {
			collectedMap[pid] = true
		}
	}

	likedIds := make([]int64, 0)
	collectedIds := make([]int64, 0)
	for _, pid := range postIds {
		if likedMap[pid] {
			likedIds = append(likedIds, pid)
		}
		if collectedMap[pid] {
			collectedIds = append(collectedIds, pid)
		}
	}

	return likedIds, collectedIds, nil
}

func joinErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
