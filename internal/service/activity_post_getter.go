package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/repo"
)

const (
	SubjectActivity = "activity"
	SubjectPost     = "post"
	SubjectComment  = "comment"
)

type SubjectInfo struct {
	Subject   string
	StudentID string
	Id       int64
}

type SubjectGetter interface {
	GetSubjectInfo(ctx context.Context, id int64, sub string) (SubjectInfo, error)
}

type subjectGetter struct {
	ad *repo.ActivityRepo
	pd *repo.PostRepo
	cd *dao.CommentDao
}

func NewSubjectGetter(ad *repo.ActivityRepo, pd *repo.PostRepo, cd *dao.CommentDao) SubjectGetter {
	return &subjectGetter{
		ad: ad,
		pd: pd,
		cd: cd,
	}
}

func (g *subjectGetter) GetSubjectInfo(ctx context.Context, id int64, sub string) (SubjectInfo, error) {
	switch sub {
	case SubjectActivity:
		act, err := g.ad.FindActById(ctx, id)
		if err != nil {
			return SubjectInfo{}, err
		}

		return SubjectInfo{
			Subject:   SubjectActivity,
			StudentID: act.StudentID,
			Id:        act.Id,
		}, nil

	case SubjectPost:
		post, err := g.pd.FindPostById(ctx, id)
		if err != nil {
			return SubjectInfo{}, err
		}

		return SubjectInfo{
			Subject:   SubjectPost,
			StudentID: post.StudentID,
			Id:        post.Id,
		}, nil

	case SubjectComment:
		cmt := g.cd.FindCmtByID(ctx, id)
		if cmt == nil {
			return SubjectInfo{}, errors.New("comment not found")
		}

		return SubjectInfo{
			Subject:   SubjectComment,
			StudentID: cmt.StudentID,
			Id:        cmt.Id,
		}, nil
	}

	return SubjectInfo{}, errors.New("invalid subject")
}