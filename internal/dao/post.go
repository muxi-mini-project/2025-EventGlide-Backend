package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PostDaoHdl interface {
	GetAllPost(ctx context.Context, page, limit int) (*model.PaginatedPosts, error)
	CreatePost(ctx context.Context, tx *gorm.DB, post *model.Post) error
	DeleteDraftByStudent(ctx context.Context, tx *gorm.DB, sid string) error
	FindPostByName(ctx context.Context, name string, page, limit int) (*model.PaginatedPosts, error)
	DeletePost(ctx context.Context, post *model.Post) error
	FindPostByUser(ctx context.Context, sid string, keyword string, page, limit int) (*model.PaginatedPosts, error)
	CreateDraft(ctx context.Context, tx *gorm.DB, draft *model.PostDraft) error
	LoadDraft(ctx context.Context, sid string) (model.PostDraft, error)
	FindPostByOwnerID(ctx context.Context, id string, page, limit int) (*model.PaginatedPosts, error)
	FindPostById(ctx context.Context, id int64) (model.Post, error)
}

type PostDao struct {
	db     *gorm.DB
	effect string
	l      *zap.Logger
}

func NewPostDao(db *gorm.DB, cfg *config.Conf, l *logger.LoggerSet) *PostDao {
	return &PostDao{
		db:     db,
		effect: cfg.Auditor.Effect,
		l:      l.Post.Named("dao"),
	}
}

func (pd *PostDao) DB() *gorm.DB {
	return pd.db
}

func (pd *PostDao) GetAllPost(ctx context.Context, page, limit int) (*model.PaginatedPosts, error) {
	var posts []model.Post
	var total int64
	offset := (page - 1) * limit

	err := pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Preload("Images").Order("created_at DESC").Limit(limit).Offset(offset).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Model(&model.Post{}).Count(&total).Error
	if err != nil {
		return nil, err
	}
	return &model.PaginatedPosts{
		Total: total,
		Page:  page,
		Limit: limit,
		Posts: posts,
	}, nil
}

func (pd *PostDao) CreatePost(ctx context.Context, tx *gorm.DB, post *model.Post) error {
	return tx.WithContext(ctx).Create(post).Error
}

func (pd *PostDao) DeleteDraftByStudent(ctx context.Context, tx *gorm.DB, sid string) error {
	return tx.WithContext(ctx).Where("student_id = ?", sid).Delete(&model.PostDraft{}).Error
}

func (pd *PostDao) FindPostByName(ctx context.Context, name string, page, limit int) (*model.PaginatedPosts, error) {
	var posts []model.Post
	var total int64
	offset := (page - 1) * limit

	err := pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Preload("Images").Where("title like ?", fmt.Sprintf("%%%s%%", name)).Order("created_at DESC").Limit(limit).Offset(offset).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Where("title like ?", fmt.Sprintf("%%%s%%", name)).Model(&model.Post{}).Count(&total).Error
	if err != nil {
		return nil, err
	}
	return &model.PaginatedPosts{
		Total: total,
		Page:  page,
		Limit: limit,
		Posts: posts,
	}, nil
}

func (pd *PostDao) DeletePost(ctx context.Context, post *model.Post) error {
	var p model.Post
	return pd.db.WithContext(ctx).Where("id = ? and student_id = ?", post.Id, post.StudentID).Delete(&p).Error
}

func (pd *PostDao) FindPostByUser(ctx context.Context, sid string, keyword string, page, limit int) (*model.PaginatedPosts, error) {
	var posts []model.Post
	var total int64
	offset := (page - 1) * limit

	var err error
	if keyword == "" {
		err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Preload("Images").Where("student_id = ?", sid).Order("created_at DESC").Limit(limit).Offset(offset).Find(&posts).Error
		if err != nil {
			return nil, err
		}
		err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Where("student_id = ?", sid).Model(&model.Post{}).Count(&total).Error
		if err != nil {
			return nil, err
		}
	} else {
		err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Preload("Images").Where("student_id = ? and title like ?", sid, fmt.Sprintf("%%%s%%", keyword)).Order("created_at DESC").Limit(limit).Offset(offset).Find(&posts).Error
		if err != nil {
			return nil, err
		}
		err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Where("student_id = ? and title like ?", sid, fmt.Sprintf("%%%s%%", keyword)).Model(&model.Post{}).Count(&total).Error
		if err != nil {
			return nil, err
		}
	}
	return &model.PaginatedPosts{
		Total: total,
		Page:  page,
		Limit: limit,
		Posts: posts,
	}, nil
}

func (pd *PostDao) CreateDraft(ctx context.Context, tx *gorm.DB, draft *model.PostDraft) error {
	return tx.WithContext(ctx).Create(draft).Error
}

func (pd *PostDao) LoadDraft(ctx context.Context, sid string) (model.PostDraft, error) {
	var draft model.PostDraft
	err := pd.db.WithContext(ctx).Preload("Images").Where("student_id = ?", sid).First(&draft).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.PostDraft{}, nil
		}
		return model.PostDraft{}, err
	}
	return draft, nil
}

func (pd *PostDao) FindPostByOwnerID(ctx context.Context, id string, page, limit int) (*model.PaginatedPosts, error) {
	var posts []model.Post
	var total int64
	offset := (page - 1) * limit

	err := pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Preload("Images").Where("student_id = ?", id).Order("created_at DESC").Limit(limit).Offset(offset).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	err = pd.db.WithContext(ctx).Scopes(pd.SetEffect()).Where("student_id = ?", id).Model(&model.Post{}).Count(&total).Error
	if err != nil {
		return nil, err
	}
	return &model.PaginatedPosts{
		Total: total,
		Page:  page,
		Limit: limit,
		Posts: posts,
	}, nil
}

func (pd *PostDao) FindPostById(ctx context.Context, id int64) (model.Post, error) {
	var post model.Post
	err := pd.db.WithContext(ctx).Preload("Images").Where("id = ?", id).First(&post).Error
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
	err := pd.db.WithContext(c).Preload("Images").Where("student_id = ? AND is_checking = ?", sid, "pending").Find(&posts).Error
	if err != nil {
		pd.l.Error("Failed to get checking posts", zap.Error(err), zap.String("student_id", sid))
		return nil, err
	}
	return posts, nil
}

func (pd *PostDao) FindPostsByIds(c context.Context, ids []int64) ([]model.Post, error) {
	if len(ids) == 0 {
		return []model.Post{}, nil
	}
	var posts []model.Post
	err := pd.db.WithContext(c).Scopes(pd.SetEffect()).Preload("Images").Where("id IN ?", ids).Find(&posts).Error
	return posts, err
}
