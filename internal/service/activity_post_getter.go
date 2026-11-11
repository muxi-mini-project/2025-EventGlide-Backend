package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
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
	// RootObjectType / RootObjectID 仅评论使用：点赞/收藏评论时据其校验所属根对象的可见性。
	RootObjectType string
	RootObjectID   int64
}

type SubjectGetter interface {
	GetSubjectInfo(ctx context.Context, id int64, sub string) (SubjectInfo, error)
	// ActivityVisibleTo reports whether the activity is visible to viewer, using the
	// same rule as the detail endpoint (service.ActivityVisible).
	ActivityVisibleTo(ctx context.Context, id int64, viewer string) (bool, error)
}

type subjectGetter struct {
	ad     *repo.ActivityRepo
	pd     *repo.PostRepo
	cd     *dao.CommentDao
	effect string
}

func NewSubjectGetter(ad *repo.ActivityRepo, pd *repo.PostRepo, cd *dao.CommentDao, cfg *config.Conf) SubjectGetter {
	return &subjectGetter{
		ad:     ad,
		pd:     pd,
		cd:     cd,
		effect: cfg.Auditor.EffectMode(),
	}
}

// ActivityVisibleTo reports whether the activity is visible to viewer.
// The rule itself lives in ActivityVisible; a missing activity is treated as not visible
// so a stale comment row cannot leak content. The approvement lookup runs only when the
// effect rule already denies access, keeping the hot read path free of an extra query.
func (g *subjectGetter) ActivityVisibleTo(ctx context.Context, id int64, viewer string) (bool, error) {
	act, err := g.ad.FindActAuditBrief(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if ActivityVisible(act.IsChecking, act.StudentID, viewer, false, g.effect) {
		return true, nil
	}
	if viewer == "" {
		return false, nil
	}
	isSigner, err := g.ad.ExistsApprovement(ctx, act.Id, viewer)
	if err != nil {
		return false, err
	}
	return ActivityVisible(act.IsChecking, act.StudentID, viewer, isSigner, g.effect), nil
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
			Subject:        SubjectComment,
			StudentID:      cmt.StudentID,
			Id:             cmt.Id,
			RootObjectType: cmt.RootObjectType,
			RootObjectID:   cmt.RootObjectID,
		}, nil
	}

	return SubjectInfo{}, errs.ErrInvalidSubject
}
