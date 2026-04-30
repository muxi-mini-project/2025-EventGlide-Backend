package repo

import (
	"context"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
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

func (r *ActivityRepo) CreateDraft(ctx context.Context, draft *model.ActivityDraft) error {
	return r.dao.CreateDraft(ctx, draft)
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

func (r *ActivityRepo) FindActByUser(ctx context.Context, sid, keyword string) ([]model.Activity, error) {
	return r.dao.FindActByUser(ctx, sid, keyword)
}

func (r *ActivityRepo) FindActByName(ctx context.Context, name string) ([]model.Activity, error) {
	return r.dao.FindActByName(ctx, name)
}

func (r *ActivityRepo) FindActByDate(ctx context.Context, date string) ([]model.Activity, error) {
	return r.dao.FindActByDate(ctx, date)
}

func (r *ActivityRepo) FindActBySearches(ctx context.Context, req *req.ActSearchReq) ([]model.Activity, error) {
	return r.dao.FindActBySearches(ctx, req)
}

func (r *ActivityRepo) FindActByOwnerID(ctx context.Context, sid string) ([]model.Activity, error) {
	return r.dao.FindActByOwnerID(ctx, sid)
}

func (r *ActivityRepo) ListAllActs(ctx context.Context) ([]model.Activity, error) {
	return r.dao.ListAllActs(ctx)
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
