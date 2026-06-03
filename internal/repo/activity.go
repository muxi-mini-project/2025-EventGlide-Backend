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

func (r *ActivityRepo) CreateAct(ctx context.Context, act *model.Activity) error {
	if err := r.dao.CreateAct(ctx, act); err != nil {
		return err
	}
	return r.Invalidate(ctx, act.Bid)
}

func (r *ActivityRepo) CreateActivityTx(ctx context.Context, act *model.Activity, signers []model.Signer, studentID string) error {
	return r.dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		act.Signers = nil
		if err := tx.Where("student_id = ?", studentID).Delete(&model.ActivityDraft{}).Error; err != nil {
			return err
		}

		if err := tx.Create(act).Error; err != nil {
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
			if err := tx.Create(&activitySigners).Error; err != nil {
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
			},
			)
		}

		if len(approvements) > 0 {
			if err := tx.Create(&approvements).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *ActivityRepo) CreateDraft(ctx context.Context, draft *model.ActivityDraft) error {
	draftSigners := draft.Signers
	draft.Signers = nil
	return r.dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var oldDrafts []model.ActivityDraft
		if err := tx.
			Where("student_id = ?", draft.StudentID).
			Find(&oldDrafts).Error; err != nil {
			return err
		}

		for _, d := range oldDrafts {
			if err := tx.
				Where("activity_bid = ?", d.Bid).
				Delete(&model.ActivitySigner{}).Error; err != nil {
				return err
			}
		}

		if err := tx.
			Where("student_id = ?", draft.StudentID).
			Delete(&model.ActivityDraft{}).Error; err != nil {
			return err
		}

		if err := tx.Create(draft).Error; err != nil {
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
			if err := tx.Create(&signers).Error; err != nil {
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
	//return cache.GetTyped(r.ch, ctx, r.actByBidKey(bid), 5*time.Minute, func(context.Context) (model.Activity, error) {
	//	act, err := r.dao.FindActByBid(ctx, bid)
	//	if err != nil {
	//		if errors.Is(err, gorm.ErrRecordNotFound) {
	//			return model.Activity{}, cache.MarkNotFound(err)
	//		}
	//		return model.Activity{}, err
	//	}
	//	return act, nil
	//})
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
