package dao

import (
	"context"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CommentDaoHdl interface {
	CreateComment(context.Context, *model.Comment) error
	DeleteComment(context.Context, string, string) error
	AnswerComment(context.Context, *model.Comment) error
	LoadComments(context.Context, string) ([]model.Comment, error)
	LoadAnswers(context.Context, string) ([]model.Comment, error)
}

type CommentDao struct {
	db *gorm.DB
	l  *zap.Logger
}

func NewCommentDao(db *gorm.DB, l *zap.Logger) *CommentDao {
	return &CommentDao{
		db: db,
		l:  l.Named("comment/dao"),
	}
}

func (cd *CommentDao) CreateComment(c context.Context, cmt *model.Comment) error {
	return cd.db.WithContext(c).Create(cmt).Error
}

func (cd *CommentDao) DeleteComment(c context.Context, sid, bid string) error {
	return cd.db.WithContext(c).Where("student_id = ? and bid = ?", sid, bid).Delete(&model.Comment{}).Error
}

func (cd *CommentDao) AnswerComment(c context.Context, cmt *model.Comment) error {
	return cd.db.WithContext(c).Create(cmt).Error
}

func (cd *CommentDao) LoadComments(c context.Context, parentid string) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("parent_id = ?", parentid).Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) LoadAnswers(c context.Context, pid string) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.WithContext(c).Where("root_id = ? and subject = 'comment'", pid).Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) FindCmtByID(c context.Context, cid string) *model.Comment {
	var cmt model.Comment
	if cd.db.WithContext(c).Where("bid = ?", cid).First(&cmt).Error != nil {
		return nil
	}
	return &cmt
}
