package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PostDaoHdl interface {
	GetAllPost(ctx context.Context) ([]model.Post, error)
	CreatePost(ctx context.Context, post *model.Post) error
	FindPostByName(ctx context.Context, name string) ([]model.Post, error)
	DeletePost(ctx context.Context, post *model.Post) error
	FindPostByUser(ctx context.Context, sid string, keyword string) ([]model.Post, error)
	CreateDraft(ctx context.Context, draft *model.PostDraft) error
	LoadDraft(ctx context.Context, bid string, sid string) (model.PostDraft, error)
	FindPostByOwnerID(ctx context.Context, id string) ([]model.Post, error)
}

type PostDao struct {
	db     *gorm.DB
	effect string
	l      *zap.Logger
}

func NewPostDao(db *gorm.DB, l *zap.Logger, cfg *config.Conf) *PostDao {
	return &PostDao{
		db:     db,
		effect: cfg.Auditor.Effect,
		l:      l.Named("post/dao"),
	}
}

func (pd *PostDao) GetAllPost(ctx context.Context) ([]model.Post, error) {
	var posts []model.Post
	err := pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return posts, nil
}

func (pd *PostDao) CreatePost(ctx context.Context, post *model.Post) error {
	pd.db.WithContext(ctx).Where("student_id = ?", post.StudentID).Delete(model.PostDraft{})
	return pd.db.WithContext(ctx).Create(post).Error
}

func (pd *PostDao) FindPostByName(ctx context.Context, name string) ([]model.Post, error) {
	var posts []model.Post
	err := pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Where("title like ?", fmt.Sprintf("%%%s%%", name)).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return posts, nil
}

func (pd *PostDao) DeletePost(ctx context.Context, post *model.Post) error {
	var p model.Post
	return pd.db.WithContext(ctx).Where("bid = ? and student_id = ?", post.Bid, post.StudentID).Delete(&p).Error
}

func (pd *PostDao) FindPostByUser(ctx context.Context, sid string, keyword string) ([]model.Post, error) {
	if keyword == "" {
		var posts []model.Post
		err := pd.db.WithContext(ctx).Where("student_id = ?", sid).Find(&posts).Error
		if err != nil {
			return nil, err
		}
		return posts, nil
	} else {
		var posts []model.Post
		err := pd.db.WithContext(ctx).Where("student_id = ? and title like ?", sid, fmt.Sprintf("%%%s%%", keyword)).Find(&posts).Error
		if err != nil {
			return nil, err
		}
		return posts, nil
	}
}

func (pd *PostDao) CreateDraft(ctx context.Context, draft *model.PostDraft) error {
	pd.db.WithContext(ctx).Where("student_id = ?", draft.StudentID).Delete(&model.PostDraft{})
	return pd.db.WithContext(ctx).Create(draft).Error
}

func (pd *PostDao) LoadDraft(ctx context.Context, sid string) (model.PostDraft, error) {
	var draft model.PostDraft
	err := pd.db.WithContext(ctx).Where("student_id = ?", sid).Find(&draft).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return draft, nil
		}
		return model.PostDraft{}, err
	}
	return draft, nil
}

func (pd *PostDao) FindPostByOwnerID(ctx context.Context, id string) ([]model.Post, error) {
	var posts []model.Post
	err := pd.db.WithContext(ctx).Where("student_id = ?", id).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return posts, nil
}

func (pd *PostDao) FindPostByBid(ctx context.Context, bid string) (model.Post, error) {
	var post model.Post
	err := pd.db.WithContext(ctx).Where("bid = ?", bid).First(&post).Error
	if err != nil {
		return model.Post{}, err
	}
	return post, nil
}

func (pd *PostDao) SetEffect() func(db *gorm.DB) *gorm.DB {
	if pd.effect == "slow" {
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("is_checking = ?", "pass")
		}
	} else if pd.effect == "fast" {
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("is_checking != ?", "reject")
		}
	}
	return func(db *gorm.DB) *gorm.DB {
		return db
	}
}

func (pd *PostDao) GetChecking(c context.Context, sid string) ([]model.Post, error) {
	var posts []model.Post
	err := pd.db.WithContext(c).Where("student_id = ? AND is_checking = ?", sid, "pending").Find(&posts).Error
	if err != nil {
		pd.l.Error("Failed to get checking posts", zap.Error(err), zap.String("student_id", sid))
		return nil, err
	}
	return posts, nil
}
