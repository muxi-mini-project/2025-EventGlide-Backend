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
	Bid       string
}

type SubjectGetter interface {
	GetSubjectInfo(ctx context.Context, bid string, sub string) (SubjectInfo, error)
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

func (g *subjectGetter) GetSubjectInfo(ctx context.Context, bid string, sub string) (SubjectInfo, error) {
	switch sub {
	case SubjectActivity:
		act, err := g.ad.FindActByBid(ctx, bid)
		if err != nil {
			return SubjectInfo{}, err
		}

		return SubjectInfo{
			Subject:   SubjectActivity,
			StudentID: act.StudentID,
			Bid:       act.Bid,
		}, nil

	case SubjectPost:
		post, err := g.pd.FindPostByBid(ctx, bid)
		if err != nil {
			return SubjectInfo{}, err
		}

		return SubjectInfo{
			Subject:   SubjectPost,
			StudentID: post.StudentID,
			Bid:       post.Bid,
		}, nil

	case SubjectComment:
		cmt := g.cd.FindCmtByID(ctx, bid)
		if cmt == nil {
			return SubjectInfo{}, errors.New("comment not found")
		}

		return SubjectInfo{
			Subject:   SubjectComment,
			StudentID: cmt.StudentID,
			Bid:       cmt.Bid,
		}, nil
	}

	return SubjectInfo{}, errors.New("invalid subject")
}
