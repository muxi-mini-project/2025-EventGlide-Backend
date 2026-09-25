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
	FindPendingAuditorPosts(ctx context.Context) ([]model.Post, error)
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
	var ids []int64
	if err := tx.WithContext(ctx).Model(&model.PostDraft{}).Where("student_id = ?", sid).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if err := deleteImagesByOwner(ctx, tx, "post_draft", ids); err != nil {
		return err
	}
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
	return pd.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? and student_id = ?", post.Id, post.StudentID).Delete(&model.Post{})
		if res.Error != nil {
			return res.Error
		}
		// 仅当帖子确属该学生（删除命中）时才清图片，避免越权删他人帖子的图片。
		if res.RowsAffected == 0 {
			return nil
		}
		return deleteImagesByOwner(ctx, tx, "post", []int64{post.Id})
	})
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

// FindPendingAuditorPosts 捞出待送审的帖子：仍为 checking 且尚无已推送的审核表单。
// 帖子在审核回调前一直保持 checking，直接按状态扫会把已推送的帖子也捞回来
// （每 tick 重复查询、并为其预加载图片），故用 NOT EXISTS 排除已推送表单
// （auditor_form 的 (activity_id, subject) 唯一索引可命中）。
func (pd *PostDao) FindPendingAuditorPosts(c context.Context) ([]model.Post, error) {
	var posts []model.Post
	err := pd.db.WithContext(c).
		Where("is_checking = ?", "checking").
		Where("NOT EXISTS (SELECT 1 FROM auditor_form af WHERE af.activity_id = post.id AND af.subject = ? AND af.pushed_at IS NOT NULL)", model.SubjectPost).
		Preload("Images").
		Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return posts, nil
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
	err := pd.db.WithContext(c).Preload("Images").Where("student_id = ? AND is_checking = ?", sid, "checking").Find(&posts).Error
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
