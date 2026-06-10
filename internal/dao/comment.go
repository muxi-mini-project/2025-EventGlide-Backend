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
	DeleteComment(context.Context, string, int64) error
	AnswerComment(context.Context, *model.Comment) error
	LoadComments(context.Context, int64) ([]model.Comment, error)
	LoadAnswers(context.Context, int64) ([]model.Comment, error)
	LoadAnswersBatch(context.Context, []int64) ([]model.Comment, error)
	FindCmtByID(context.Context, int64) *model.Comment
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

func (cd *CommentDao) DeleteComment(c context.Context, sid string, id int64) error {
	return cd.db.WithContext(c).Where("student_id = ? and id = ?", sid, id).Delete(&model.Comment{}).Error
}

func (cd *CommentDao) AnswerComment(c context.Context, cmt *model.Comment) error {
	return cd.db.WithContext(c).Create(cmt).Error
}

func (cd *CommentDao) LoadComments(c context.Context, parentId int64) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("parent_id = ?", parentId).Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) LoadAnswers(c context.Context, rootId int64) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("root_id = ? and subject = 'comment'", rootId).Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) FindCmtByID(c context.Context, id int64) *model.Comment {
	var cmt model.Comment
	if cd.db.WithContext(c).Where("id = ?", id).First(&cmt).Error != nil {
		return nil
	}
	return &cmt
}

func (cd *CommentDao) LoadAnswersBatch(c context.Context, rootIDs []int64) ([]model.Comment, error) {
	if len(rootIDs) == 0 {
		return nil, nil
	}
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("root_id IN ? AND subject = 'comment'", rootIDs).Find(&cmts).Error
	return cmts, err
}

