package repo

import (
	"context"

	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
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

func (r *PostRepo) GetAllPost(ctx context.Context, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.GetAllPost(ctx, page, limit)
}

func (r *PostRepo) CreatePost(ctx context.Context, post *model.Post) error {
	if err := r.dao.CreatePost(ctx, post); err != nil {
		return err
	}
	return r.Invalidate(ctx, post.Bid)
}

func (r *PostRepo) FindPostByName(ctx context.Context, name string, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.FindPostByName(ctx, name, page, limit)
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

func (r *PostRepo) FindPostByUser(ctx context.Context, sid, keyword string, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.FindPostByUser(ctx, sid, keyword, page, limit)
}

func (r *PostRepo) CreateDraft(ctx context.Context, draft *model.PostDraft) error {
	return r.dao.CreateDraft(ctx, draft)
}

func (r *PostRepo) LoadDraft(ctx context.Context, sid string) (model.PostDraft, error) {
	return r.dao.LoadDraft(ctx, sid)
}

func (r *PostRepo) FindPostByOwnerID(ctx context.Context, id string, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.FindPostByOwnerID(ctx, id, page, limit)
}

func (r *PostRepo) FindPostByBid(ctx context.Context, bid string) (model.Post, error) {
	return r.dao.FindPostByBid(ctx, bid)
}

func (r *PostRepo) GetChecking(ctx context.Context, sid string) ([]model.Post, error) {
	return r.dao.GetChecking(ctx, sid)
}

func (r *PostRepo) Invalidate(ctx context.Context, bid string) error {
	return r.ch.SetAndInvalidate(ctx, r.postByBidKey(bid), nil, 0)
}

func (r *PostRepo) postByBidKey(bid string) string {
	return r.kb.Build("bid", bid)
}
