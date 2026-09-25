package repo

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
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

// DeletePostInteractionCache 清掉帖子在 Redis 的点赞/收藏 Set 与计数 key（帖子被删时调用）。
func (r *InteractionRepo) DeletePostInteractionCache(ctx context.Context, postID int64) error {
	return r.lfr.DeleteInteractions(ctx, cache.SubjectPost, []int64{postID})
}

// DeleteCommentInteractionCache 清掉评论在 Redis 的点赞 Set 与计数 key
// （删帖级联删评论时调用），一次 DEL 批量清理。
func (r *InteractionRepo) DeleteCommentInteractionCache(ctx context.Context, commentIDs []int64) error {
	return r.lfr.DeleteInteractions(ctx, cache.SubjectComment, commentIDs)
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

// DecreaseActivityCommentNum 回减活动评论数并失效对应缓存。
func (r *InteractionRepo) DecreaseActivityCommentNum(ctx context.Context, activityId int64, n int64) error {
	if err := r.dao.DecreaseActivityCommentNum(ctx, activityId, n); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, activityId)
}

// DecreasePostCommentNum 回减帖子评论数并失效对应缓存。
func (r *InteractionRepo) DecreasePostCommentNum(ctx context.Context, postId int64, n int64) error {
	if err := r.dao.DecreasePostCommentNum(ctx, postId, n); err != nil {
		return err
	}
	return r.posts.Invalidate(ctx, postId)
}

func (r *InteractionRepo) ApproveActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.ApproveActivity(ctx, studentID, targetID); err != nil {
		return mapSignerDecisionErr(err)
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) RejectActivity(ctx context.Context, studentID string, targetID int64) error {
	if err := r.dao.RejectActivity(ctx, studentID, targetID); err != nil {
		return mapSignerDecisionErr(err)
	}
	return r.acts.Invalidate(ctx, targetID)
}

func mapSignerDecisionErr(err error) error {
	switch {
	case errors.Is(err, dao.ErrSignerDecisionNotAllowed):
		return errs.ErrInteractionNotAllowed.Wrap(err)
	case errors.Is(err, dao.ErrSignerNotApprover):
		return errs.ErrForbidden.Wrap(err)
	default:
		return err
	}
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

// GetUserPostInteractionMaps 批量获取用户对多个帖子的点赞+收藏状态（Redis 单次 pipeline，异常降级一次批量 SQL）
func (r *InteractionRepo) GetUserPostInteractionMaps(ctx context.Context, userId int64, postIds []int64) (likedMap, collectedMap map[int64]bool, err error) {
	if len(postIds) == 0 {
		return make(map[int64]bool), make(map[int64]bool), nil
	}
	likedMap, collectedMap, err = r.lfr.MGetUserInteractionStatus(ctx, cache.SubjectPost, postIds, userId)
	if err == nil {
		return likedMap, collectedMap, nil
	}
	likedIds, collectedIds, err := r.dao.GetUserPostInteractionStatuses(ctx, userId, postIds)
	if err != nil {
		return nil, nil, err
	}
	likedMap = make(map[int64]bool, len(postIds))
	collectedMap = make(map[int64]bool, len(postIds))
	for _, id := range postIds {
		likedMap[id] = false
		collectedMap[id] = false
	}
	for _, id := range likedIds {
		likedMap[id] = true
	}
	for _, id := range collectedIds {
		collectedMap[id] = true
	}
	return likedMap, collectedMap, nil
}

// GetUserLikedCommentMap 批量获取用户对多条评论的点赞状态（Redis 单次 pipeline，异常降级批量 SQL）
func (r *InteractionRepo) GetUserLikedCommentMap(ctx context.Context, userId int64, commentIds []int64) (map[int64]bool, error) {
	if len(commentIds) == 0 {
		return make(map[int64]bool), nil
	}
	likedMap, err := r.lfr.MGetLikedStatusesForUser(ctx, cache.SubjectComment, commentIds, userId)
	if err == nil {
		return likedMap, nil
	}
	// Redis 异常降级：MySQL 批量查
	likedIds, err := r.dao.GetUserLikedCommentIds(ctx, userId, commentIds)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]bool, len(commentIds))
	for _, id := range commentIds {
		result[id] = false
	}
	for _, id := range likedIds {
		result[id] = true
	}
	return result, nil
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

	likedMap, collectedMap, err := r.lfr.MGetUserInteractionStatus(ctx, cache.SubjectActivity, activityIds, userId)
	if err != nil {
		return r.dao.GetUserActivityInteractionStatuses(ctx, userId, activityIds)
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

	likedMap, collectedMap, err := r.lfr.MGetUserInteractionStatus(ctx, cache.SubjectPost, postIds, userId)
	if err != nil {
		return r.dao.GetUserPostInteractionStatuses(ctx, userId, postIds)
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
