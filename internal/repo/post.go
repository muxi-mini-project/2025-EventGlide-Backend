package repo

import (
	"context"
	"fmt"

	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PostRepo struct {
	dao *dao.PostDao
	ch  *cache.MultiLevelCache
	kb  cache.KeyBuilder
	l   *zap.Logger
}

func NewPostRepo(dao *dao.PostDao, ch *cache.MultiLevelCache, l *logger.LoggerSet) *PostRepo {
	return &PostRepo{
		dao: dao,
		ch:  ch,
		kb:  cache.NewKeyBuilder("post"),
		l:   l.Post.Named("repo"),
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

func (r *PostRepo) DeletePost(ctx context.Context, post *model.Post) (bool, []int64, error) {
	deleted, commentIDs, err := r.dao.DeletePost(ctx, post)
	if err != nil {
		return false, nil, err
	}
	if !deleted || post.Id == 0 {
		return deleted, commentIDs, nil
	}
	// 缓存失效尽力而为：DB 删除已提交，失效失败只记录，不改变删除结果，
	// 也不阻断 service 后续的互动缓存清理。
	if err := r.Invalidate(ctx, post.Id); err != nil {
		r.l.Error("Failed to invalidate post cache", zap.Error(err), zap.Int64("id", post.Id))
	}
	return deleted, commentIDs, nil
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

func (r *PostRepo) FindPendingAuditorPosts(ctx context.Context) ([]model.Post, error) {
	return r.dao.FindPendingAuditorPosts(ctx)
}
