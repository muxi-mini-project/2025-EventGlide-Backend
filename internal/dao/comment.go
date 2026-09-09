package dao

import (
	"context"

	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CommentDaoHdl interface {
	CreateComment(context.Context, *model.Comment) error
	DeleteCommentCascade(context.Context, string, int64) (int64, error)
	FindCmtByIDNoDecrypt(context.Context, int64) (*model.Comment, error)
	AnswerComment(context.Context, *model.Comment) error
	LoadComments(context.Context, int64) ([]model.Comment, error)
	LoadAnswers(context.Context, int64) ([]model.Comment, error)
	LoadAnswersBatch(context.Context, []int64) ([]model.Comment, error)
	FindCmtByID(context.Context, int64) (*model.Comment, error)
}

type CommentDao struct {
	db *gorm.DB
	l  *zap.Logger
}

func NewCommentDao(db *gorm.DB, l *logger.LoggerSet) *CommentDao {
	return &CommentDao{
		db: db,
		l:  l.Comment.Named("comment"),
	}
}

func (cd *CommentDao) CreateComment(c context.Context, cmt *model.Comment) error {
	return cd.db.WithContext(c).Create(cmt).Error
}

// DeleteCommentCascade 删除本人评论：若为一级评论则级联删除整个扁平子树
// （root_id 指向它的全部子级回复），子级评论只删自身（兄弟回复平铺展示不受影响）。
// 返回本次实际删除的条数；仅当本人评论存在时才级联，权限校验由外层 sid 保证。
func (cd *CommentDao) DeleteCommentCascade(c context.Context, sid string, id int64) (int64, error) {
	deleted := int64(0)
	err := cd.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("student_id = ? and id = ?", sid, id).Delete(&model.Comment{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 本人评论不存在，无可删除（不视为错误）
			return nil
		}
		deleted = res.RowsAffected

		// 一级子树：root_id 指向它的全部子级（子级自身删除时此条件无命中，自然只删自己）
		sub := tx.Where("root_id = ? AND subject = 'comment'", id).Delete(&model.Comment{})
		if sub.Error != nil {
			return sub.Error
		}
		deleted += sub.RowsAffected
		return nil
	})
	return deleted, err
}

// FindCmtByIDNoDecrypt 按 ID 查评论的归属信息（只取非加密列，容忍加密字段损坏）。
// 删除流程用它取 root 归属，损坏行同样允许删除。
func (cd *CommentDao) FindCmtByIDNoDecrypt(c context.Context, id int64) (*model.Comment, error) {
	var cmt model.Comment
	err := cd.db.WithContext(c).
		Select("id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type").
		Where("id = ?", id).First(&cmt).Error
	if err != nil {
		return nil, err
	}
	return &cmt, nil
}

func (cd *CommentDao) AnswerComment(c context.Context, cmt *model.Comment) error {
	return cd.db.WithContext(c).Create(cmt).Error
}

// LoadComments 加载某活动/帖子下的一级评论（subject 限定非 comment，防止误传子级评论 ID）
func (cd *CommentDao) LoadComments(c context.Context, parentId int64) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("parent_id = ? AND subject <> 'comment'", parentId).
		Order("created_at ASC").Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) LoadAnswers(c context.Context, rootId int64) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("root_id = ? and subject = 'comment'", rootId).
		Order("created_at ASC").Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) FindCmtByID(c context.Context, id int64) (*model.Comment, error) {
	var cmt model.Comment
	if err := cd.db.WithContext(c).Where("id = ?", id).First(&cmt).Error; err != nil {
		return nil, err
	}
	return &cmt, nil
}

func (cd *CommentDao) LoadAnswersBatch(c context.Context, rootIDs []int64) ([]model.Comment, error) {
	if len(rootIDs) == 0 {
		return nil, nil
	}
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("root_id IN ? AND subject = 'comment'", rootIDs).
		Order("created_at ASC").Find(&cmts).Error
	return cmts, err
}
