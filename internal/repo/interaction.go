package repo

import (
	"github.com/gin-gonic/gin"
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

func (r *InteractionRepo) LikeActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.LikeActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) LikePost(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.LikePost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) LikeComment(ctx *gin.Context, studentID, targetID string) error {
	return r.dao.LikeComment(ctx, studentID, targetID)
}

func (r *InteractionRepo) DislikeActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.DislikeActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DislikePost(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.DislikePost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DislikeComment(ctx *gin.Context, studentID, targetID string) error {
	return r.dao.DislikeComment(ctx, studentID, targetID)
}

func (r *InteractionRepo) CommentActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.CommentActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) CommentPost(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.CommentPost(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.posts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) CommentComment(ctx *gin.Context, studentID, targetID string) error {
	return r.dao.CommentComment(ctx, studentID, targetID)
}

func (r *InteractionRepo) CollectActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.CollectActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) CollectPost(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.CollectPost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DiscollectActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.DiscollectActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.acts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) DiscollectPost(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.DiscollectPost(ctx, studentID, targetID); err != nil {
		return err
	}
	return joinErr(
		r.users.Invalidate(ctx, studentID),
		r.posts.Invalidate(ctx, targetID),
	)
}

func (r *InteractionRepo) ApproveActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.ApproveActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) RejectActivity(ctx *gin.Context, studentID, targetID string) error {
	if err := r.dao.RejectActivity(ctx, studentID, targetID); err != nil {
		return err
	}
	return r.acts.Invalidate(ctx, targetID)
}

func (r *InteractionRepo) InsertApprovement(ctx *gin.Context, studentID, studentName, targetID string) error {
	return r.dao.InsertApprovement(ctx, studentID, studentName, targetID)
}

func joinErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
