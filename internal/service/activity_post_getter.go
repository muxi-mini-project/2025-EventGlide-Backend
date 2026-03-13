package service

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
)

const (
	SubjectActivity = "activity"
	SubjectPost     = "post"
	SubjectComment  = "comment"
)

type ActPostCommentWrapper struct {
	Activity *model.Activity
	Post     *model.Post
	Comment  *model.Comment
}

func (apw *ActPostCommentWrapper) GetStudentID() string {
	if apw.Activity != nil {
		return apw.Activity.StudentID
	}
	if apw.Post != nil {
		return apw.Post.StudentID
	}
	if apw.Comment != nil {
		return apw.Comment.StudentID
	}
	return ""
}

func (apw *ActPostCommentWrapper) GetBid() string {
	if apw.Activity != nil {
		return apw.Activity.Bid
	}
	if apw.Post != nil {
		return apw.Post.Bid
	}
	if apw.Comment != nil {
		return apw.Comment.Bid
	}
	return ""
}

type ActPostCommentGetter interface {
	GetActivityOrPostOrComment(ctx *gin.Context, bid string, sub string) (ActPostCommentWrapper, error)
}

type actPostCommentGetter struct {
	ad *dao.ActDao
	pd *dao.PostDao
	cd *dao.CommentDao
}

func NewActPostCommentGetter(ad *dao.ActDao, pd *dao.PostDao, cd *dao.CommentDao) ActPostCommentGetter {
	return &actPostCommentGetter{
		ad: ad,
		cd: cd,
		pd: pd,
	}
}

func (apg *actPostCommentGetter) GetActivityOrPostOrComment(ctx *gin.Context, bid string, sub string) (ActPostCommentWrapper, error) {
	var wrapper ActPostCommentWrapper
	switch sub {
	case SubjectActivity:
		act, err := apg.ad.FindActByBid(ctx, bid)
		if err != nil {
			return ActPostCommentWrapper{}, err
		}
		wrapper.Activity = &act
	case SubjectPost:
		post, err := apg.pd.FindPostByBid(ctx, bid)
		if err != nil {
			return ActPostCommentWrapper{}, err
		}
		wrapper.Post = &post
	case SubjectComment:
		cmt := apg.cd.FindCmtByID(ctx, bid)
		if cmt == nil { // todo 加一个错误返回
			return ActPostCommentWrapper{}, errors.New("comment not found")
		}
		wrapper.Comment = cmt
	}
	return wrapper, nil
}
