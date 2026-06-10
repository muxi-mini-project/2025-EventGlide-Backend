package repo

import (
	"context"
	"fmt"

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

func (r *PostRepo) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.dao.DB().WithContext(ctx).Transaction(fn)
}

func (r *PostRepo) GetAllPost(ctx context.Context, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.GetAllPost(ctx, page, limit)
}

func (r *PostRepo) CreatePost(ctx context.Context, post *model.Post) error {
	if err := r.Transaction(ctx, func(tx *gorm.DB) error {
		if err := r.dao.DeleteDraftByStudent(ctx, tx, post.StudentID); err != nil {
			return err
		}
		return r.dao.CreatePost(ctx, tx, post)
	}); err != nil {
		return err
	}
	return r.Invalidate(ctx, post.Id)
}

func (r *PostRepo) FindPostByName(ctx context.Context, name string, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.FindPostByName(ctx, name, page, limit)
}

func (r *PostRepo) DeletePost(ctx context.Context, post *model.Post) error {
	if err := r.dao.DeletePost(ctx, post); err != nil {
		return err
	}
	if post.Id == 0 {
		return nil
	}
	return r.Invalidate(ctx, post.Id)
}

func (r *PostRepo) FindPostByUser(ctx context.Context, sid, keyword string, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.FindPostByUser(ctx, sid, keyword, page, limit)
}

func (r *PostRepo) CreateDraft(ctx context.Context, draft *model.PostDraft) error {
	return r.Transaction(ctx, func(tx *gorm.DB) error {
		if err := r.dao.DeleteDraftByStudent(ctx, tx, draft.StudentID); err != nil {
			return err
		}
		return r.dao.CreateDraft(ctx, tx, draft)
	})
}

func (r *PostRepo) LoadDraft(ctx context.Context, sid string) (model.PostDraft, error) {
	return r.dao.LoadDraft(ctx, sid)
}

func (r *PostRepo) FindPostByOwnerID(ctx context.Context, id string, page, limit int) (*model.PaginatedPosts, error) {
	return r.dao.FindPostByOwnerID(ctx, id, page, limit)
}

func (r *PostRepo) FindPostById(ctx context.Context, id int64) (model.Post, error) {
	return r.dao.FindPostById(ctx, id)
}

func (r *PostRepo) GetChecking(ctx context.Context, sid string) ([]model.Post, error) {
	return r.dao.GetChecking(ctx, sid)
}

func (r *PostRepo) Invalidate(ctx context.Context, id int64) error {
	return r.ch.SetAndInvalidate(ctx, r.postByIdKey(id), nil, 0)
}

func (r *PostRepo) postByIdKey(id int64) string {
	return r.kb.Build("id", fmt.Sprintf("%d", id))
}

func (r *PostRepo) FindPostsByIds(ctx context.Context, ids []int64) ([]model.Post, error) {
	return r.dao.FindPostsByIds(ctx, ids)
}