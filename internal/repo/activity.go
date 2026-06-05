package repo

import (
	"context"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"gorm.io/gorm"
)

type ActivityRepo struct {
	dao *dao.ActDao
	ch  *cache.MultiLevelCache
	kb  cache.KeyBuilder
}

func NewActivityRepo(dao *dao.ActDao, ch *cache.MultiLevelCache) *ActivityRepo {
	return &ActivityRepo{
		dao: dao,
		ch:  ch,
		kb:  cache.NewKeyBuilder("activity"),
	}
}

func (r *ActivityRepo) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.dao.DB().WithContext(ctx).Transaction(fn)
}

func (r *ActivityRepo) CreateActivity(ctx context.Context, tx *gorm.DB, act *model.Activity, signers []model.Signer, studentID string) error {
	act.Signers = nil
	if err := r.dao.DeleteActivityDraft(ctx, tx, studentID); err != nil {
		return err
	}

	if err := r.dao.CreateActivity(ctx, tx, act); err != nil {
		return err
	}

	activitySigners := make([]model.ActivitySigner, 0, len(signers))
	for _, s := range signers {
		activitySigners = append(activitySigners, model.ActivitySigner{
			ActivityBid: act.Bid,
			StudentID:   s.StudentID,
			Name:        s.Name,
		})
	}
	if len(activitySigners) > 0 {
		if err := r.dao.CreateActivitySigners(ctx, tx, activitySigners); err != nil {
			return err
		}
	}

	approvements := make([]model.Approvement, 0, len(signers))
	for _, s := range signers {
		if s.StudentID == studentID {
			continue
		}
		approvements = append(approvements, model.Approvement{
			StudentId:   s.StudentID,
			StudentName: s.Name,
			Bid:         act.Bid,
		})
	}

	if len(approvements) > 0 {
		if err := r.dao.CreateApprovements(ctx, tx, approvements); err != nil {
			return err
		}
	}

	return nil
}

func (r *ActivityRepo) CreateDraft(ctx context.Context, draft *model.ActivityDraft) error {
	return r.dao.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		draftSigners := draft.Signers
		draftCopy := *draft
		draftCopy.Signers = nil

		var oldDrafts []model.ActivityDraft
		if err := r.dao.FindDraftsByStudentID(ctx, tx, draft.StudentID, &oldDrafts); err != nil {
			return err
		}

		for _, d := range oldDrafts {
			if err := r.dao.DeleteSignersByActivityBid(ctx, tx, d.Bid); err != nil {
				return err
			}
		}

		if err := r.dao.DeleteDraftsByStudentID(ctx, tx, draft.StudentID); err != nil {
			return err
		}

		if err := r.dao.CreateDraft(ctx, tx, &draftCopy); err != nil {
			return err
		}

		if len(draftSigners) > 0 {
			signers := make([]model.ActivitySigner, 0, len(draftSigners))
			for _, s := range draftSigners {
				signers = append(signers, model.ActivitySigner{
					ActivityBid: draft.Bid,
					StudentID:   s.StudentID,
					Name:        s.Name,
				})
			}
			if err := r.dao.BatchCreateSigners(ctx, tx, signers); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *ActivityRepo) DeleteAct(ctx context.Context, act model.Activity) error {
	if err := r.dao.DeleteAct(ctx, act); err != nil {
		return err
	}
	if act.Bid == "" {
		return nil
	}
	return r.Invalidate(ctx, act.Bid)
}

func (r *ActivityRepo) LoadDraft(ctx context.Context, sid string) (model.ActivityDraft, error) {
	return r.dao.LoadDraft(ctx, sid)
}

func (r *ActivityRepo) FindActByUser(ctx context.Context, sid, keyword string, page, limit int) (*model.PaginatedActivities, error) {
	return r.dao.FindActByUser(ctx, sid, keyword, page, limit)
}

func (r *ActivityRepo) FindActByName(ctx context.Context, name string, page, limit int) (*model.PaginatedActivities, error) {
	return r.dao.FindActByName(ctx, name, page, limit)
}

func (r *ActivityRepo) FindActByDate(ctx context.Context, date string, page, limit int) (*model.PaginatedActivities, error) {
	return r.dao.FindActByDate(ctx, date, page, limit)
}

func (r *ActivityRepo) FindActBySearches(ctx context.Context, req *req.ActSearchReq) (*model.PaginatedActivities, error) {
	return r.dao.FindActBySearches(ctx, req)
}

func (r *ActivityRepo) FindActByOwnerID(ctx context.Context, sid string, page, limit int) (*model.PaginatedActivities, error) {
	return r.dao.FindActByOwnerID(ctx, sid, page, limit)
}

func (r *ActivityRepo) ListAllActs(ctx context.Context, page, limit int) (*model.PaginatedActivities, error) {
	return r.dao.ListAllActs(ctx, page, limit)
}

func (r *ActivityRepo) FindActByBid(ctx context.Context, bid string) (model.Activity, error) {
	return r.dao.FindActByBid(ctx, bid)
}

func (r *ActivityRepo) GetChecking(ctx context.Context, sid string) ([]model.Activity, error) {
	return r.dao.GetChecking(ctx, sid)
}

func (r *ActivityRepo) Invalidate(ctx context.Context, bid string) error {
	return r.ch.SetAndInvalidate(ctx, r.actByBidKey(bid), nil, 0)
}

func (r *ActivityRepo) actByBidKey(bid string) string {
	return r.kb.Build("bid", bid)
}
