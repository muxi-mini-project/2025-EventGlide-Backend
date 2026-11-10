package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"gorm.io/gorm"
)

const (
	SubjectActivity = "activity"
	SubjectPost     = "post"
	SubjectComment  = "comment"
)

type SubjectInfo struct {
	Subject   string
	StudentID string
	Id        int64
}

type SubjectGetter interface {
	GetSubjectInfo(ctx context.Context, id int64, sub string) (SubjectInfo, error)
	// ActivityVisibleTo mirrors the activity detail rule: a rejected (taken-down)
	// activity is only visible to its creator and signers.
	ActivityVisibleTo(ctx context.Context, id int64, viewer string) (bool, error)
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

// ActivityVisibleTo reports whether the activity is visible to viewer. It mirrors the detail
// endpoint: a rejected (taken-down) activity is only visible to its creator and signers.
// A missing activity is treated as not visible so a stale comment row cannot leak content.
func (g *subjectGetter) ActivityVisibleTo(ctx context.Context, id int64, viewer string) (bool, error) {
	act, err := g.ad.FindActAuditBrief(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if act.IsChecking != model.StanceReject {
		return true, nil
	}
	if viewer == "" {
		return false, nil
	}
	if act.StudentID == viewer {
		return true, nil
	}
	return g.ad.ExistsApprovement(ctx, act.Id, viewer)
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
		cmt, err := g.cd.FindCmtByID(ctx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return SubjectInfo{}, errs.ErrCommentNotFound
			}
			return SubjectInfo{}, err
		}

		return SubjectInfo{
			Subject:   SubjectComment,
			StudentID: cmt.StudentID,
			Id:        cmt.Id,
		}, nil
	}

	return SubjectInfo{}, errs.ErrInvalidSubject
}
