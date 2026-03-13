package dao

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CommentDaoHdl interface {
	CreateComment(*gin.Context, *model.Comment) error
	DeleteComment(*gin.Context, string, string) error
	AnswerComment(*gin.Context, *model.Comment) error
	LoadComments(*gin.Context, string) ([]model.Comment, error)
	LoadAnswers(*gin.Context, string) ([]model.Comment, error)
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

func (cd *CommentDao) CreateComment(c *gin.Context, cmt *model.Comment) error {
	return cd.db.Create(cmt).Error
}

func (cd *CommentDao) DeleteComment(c *gin.Context, sid, bid string) error {
	return cd.db.Where("student_id = ? and bid = ?", sid, bid).Delete(&model.Comment{}).Error
}

func (cd *CommentDao) AnswerComment(c *gin.Context, cmt *model.Comment) error {
	return cd.db.Create(cmt).Error
}

func (cd *CommentDao) LoadComments(c *gin.Context, parentid string) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.Where("parent_id = ?", parentid).Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) LoadAnswers(c *gin.Context, pid string) ([]model.Comment, error) {
	var cmts []model.Comment
	err := cd.db.Where("root_id = ? and subject = 'comment'", pid).Find(&cmts).Error
	return cmts, err
}

func (cd *CommentDao) FindCmtByID(c *gin.Context, cid string) *model.Comment {
	var cmt model.Comment
	if cd.db.Where("bid = ?", cid).First(&cmt).Error != nil {
		return nil
	}
	return &cmt
}
