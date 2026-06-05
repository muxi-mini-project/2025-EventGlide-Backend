package repo

import (
	"context"

	"github.com/raiki02/EG/internal/dao"
)

type InteractionRepo struct {
	dao   *dao.InteractionDao
	users *UserRepo
	acts  *ActivityRepo
	posts *PostRepo
}

func NewInteractionRepo(dao *dao.InteractionDao, users *UserRepo, acts *ActivityRepo, posts *PostRepo) *InteractionRepo {
	return &InteractionRepo{
		dao:   dao,
		users: users,
		acts:  acts,
		posts: posts,
	}
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

func (r *InteractionRepo) CommentComment(ctx context.Context, studentID string, targetID int64) error {
	return r.dao.CommentComment(ctx, studentID, targetID)
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
	return r.dao.IsUserLikedActivity(ctx, userId, activityId)
}

func (r *InteractionRepo) IsUserCollectedActivity(ctx context.Context, userId, activityId int64) bool {
	return r.dao.IsUserCollectedActivity(ctx, userId, activityId)
}

func (r *InteractionRepo) IsUserLikedPost(ctx context.Context, userId, postId int64) bool {
	return r.dao.IsUserLikedPost(ctx, userId, postId)
}

func (r *InteractionRepo) IsUserCollectedPost(ctx context.Context, userId, postId int64) bool {
	return r.dao.IsUserCollectedPost(ctx, userId, postId)
}

func (r *InteractionRepo) IsUserLikedComment(ctx context.Context, userId, commentId int64) bool {
	return r.dao.IsUserLikedComment(ctx, userId, commentId)
}

func (r *InteractionRepo) GetUserCollectedActivityIds(ctx context.Context, userId int64) ([]int64, error) {
	return r.dao.GetUserCollectedActivityIds(ctx, userId)
}

func (r *InteractionRepo) GetUserLikedActivityIds(ctx context.Context, userId int64) ([]int64, error) {
	return r.dao.GetUserLikedActivityIds(ctx, userId)
}

func (r *InteractionRepo) GetUserCollectedPostIds(ctx context.Context, userId int64) ([]int64, error) {
	return r.dao.GetUserCollectedPostIds(ctx, userId)
}

func (r *InteractionRepo) GetUserLikedPostIds(ctx context.Context, userId int64) ([]int64, error) {
	return r.dao.GetUserLikedPostIds(ctx, userId)
}

func joinErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}