package repo

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"gorm.io/gorm"
)

type PostRepo struct {
	dao *dao.PostDao
	ch  *cache.MultiLevelCache
	kb  cache.KeyBuilder
}

func NewPostRepo(dao *dao.PostDao, ch *cache.MultiLevelCache) *PostRepo {
	return &PostRepo{
		dao: dao,
		ch:  ch,
		kb:  cache.NewKeyBuilder("post"),
	}
}

func (r *PostRepo) GetAllPost(ctx context.Context) ([]model.Post, error) {
	return r.dao.GetAllPost(ctx)
}

func (r *PostRepo) CreatePost(ctx context.Context, post *model.Post) error {
	if err := r.dao.CreatePost(ctx, post); err != nil {
		return err
	}
	return r.Invalidate(ctx, post.Bid)
}

func (r *PostRepo) FindPostByName(ctx context.Context, name string) ([]model.Post, error) {
	return r.dao.FindPostByName(ctx, name)
}

func (r *PostRepo) DeletePost(ctx context.Context, post *model.Post) error {
	if err := r.dao.DeletePost(ctx, post); err != nil {
		return err
	}
	if post.Bid == "" {
		return nil
	}
	return r.Invalidate(ctx, post.Bid)
}

func (r *PostRepo) FindPostByUser(ctx context.Context, sid, keyword string) ([]model.Post, error) {
	return r.dao.FindPostByUser(ctx, sid, keyword)
}

func (r *PostRepo) CreateDraft(ctx context.Context, draft *model.PostDraft) error {
	return r.dao.CreateDraft(ctx, draft)
}

func (r *PostRepo) LoadDraft(ctx context.Context, sid string) (model.PostDraft, error) {
	return r.dao.LoadDraft(ctx, sid)
}

func (r *PostRepo) FindPostByOwnerID(ctx context.Context, id string) ([]model.Post, error) {
	return r.dao.FindPostByOwnerID(ctx, id)
}

func (r *PostRepo) FindPostByBid(ctx *gin.Context, bid string) (model.Post, error) {
	return cache.GetTyped(r.ch, ctx, r.postByBidKey(bid), 5*time.Minute, func(context.Context) (model.Post, error) {
		post, err := r.dao.FindPostByBid(ctx, bid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.Post{}, cache.MarkNotFound(err)
			}
			return model.Post{}, err
		}
		return post, nil
	})
}

func (r *PostRepo) GetChecking(ctx *gin.Context, sid string) ([]model.Post, error) {
	return r.dao.GetChecking(ctx, sid)
}

func (r *PostRepo) Invalidate(ctx context.Context, bid string) error {
	return r.ch.SetAndInvalidate(ctx, r.postByBidKey(bid), nil, 0)
}

func (r *PostRepo) postByBidKey(bid string) string {
	return r.kb.Build("bid", bid)
}
