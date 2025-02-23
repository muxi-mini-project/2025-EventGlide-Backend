package service

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
	"time"
)

type ActivityServiceHdl interface {
	NewAct(*gin.Context, *model.Activity) error
	NewDraft(*gin.Context, *model.ActivityDraft) error
	LoadDraft(*gin.Context, req.DraftReq) (*model.ActivityDraft, error)
	FindActBySearches(*gin.Context, *req.ActSearchReq) ([]model.Activity, error)
	FindActByDate(*gin.Context, string) ([]model.Activity, error)
	FindActByName(*gin.Context, string) ([]model.Activity, error)
}

type ActivityService struct {
	ad *dao.ActDao
	ch *cache.Cache
	pd *dao.PostDao
}

func NewActivityService(ad *dao.ActDao, ch *cache.Cache, pd *dao.PostDao) *ActivityService {
	return &ActivityService{
		ad: ad,
		ch: ch,
		pd: pd,
	}
}

func (as *ActivityService) NewAct(c *gin.Context, act *model.Activity) error {
	act.Bid = tools.GenUUID(c)

	createdAt := time.Now()
	act.CreatedAt = createdAt
	err := as.ad.CreateAct(c, act)
	if err != nil {
		return err
	}
	post := &model.Post{
		Bid:       act.Bid,
		CreatorID: act.CreatorId,
		Content:   act.Description,
		ImgUrls:   act.ImgUrls,
		Title:     act.Name,
		CreatedAt: createdAt,
	}
	err = as.pd.CreatePost(c, post)
	if err != nil {
		return err
	}
	return nil

}

func (as *ActivityService) NewDraft(c *gin.Context, d *model.ActivityDraft) error {
	d.Bid = tools.GenUUID(c)
	err := as.ad.CreateDraft(c, d)
	if err != nil {
		return err
	}
	return nil
}

func (as *ActivityService) LoadDraft(c *gin.Context, dReq req.DraftReq) (model.ActivityDraft, error) {
	d, err := as.ad.LoadDraft(c, dReq.Sid, dReq.Bid)
	if err != nil {
		return model.ActivityDraft{}, err
	}
	return d, nil
}

func (as *ActivityService) FindActBySearches(ctx *gin.Context, req *req.ActSearchReq) ([]model.Activity, error) {
	acts, err := as.ad.FindActBySearches(ctx, req)
	if err != nil {
		return nil, err
	}
	return acts, nil
}

func (as *ActivityService) FindActByDate(c *gin.Context, date string) ([]model.Activity, error) {
	acts, err := as.ad.FindActByDate(c, date)
	if err != nil {
		return nil, err
	}
	return acts, nil
}

func (as *ActivityService) FindActByName(c *gin.Context, name string) ([]model.Activity, error) {
	acts, err := as.ad.FindActByName(c, name)
	if err != nil {
		return nil, err
	}
	return acts, nil
}
