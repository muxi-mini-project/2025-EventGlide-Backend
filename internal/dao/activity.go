package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ ActDaoHdl = &ActDao{}

type ActDaoHdl interface {
	CreateAct(context.Context, *model.Activity) error
	CreateDraft(context.Context, *model.ActivityDraft) error
	LoadDraft(context.Context, string) (model.ActivityDraft, error)
	DeleteAct(context.Context, model.Activity) error
	ListAllActs(context.Context, int, int) (*model.PaginatedActivities, error)
	FindActByUser(context.Context, string, string, int, int) (*model.PaginatedActivities, error)
	FindActByName(context.Context, string, int, int) (*model.PaginatedActivities, error)
	FindActByDate(context.Context, string, int, int) (*model.PaginatedActivities, error)
	FindActByOwnerID(context.Context, string, int, int) (*model.PaginatedActivities, error)
	FindActBySearches(context.Context, *req.ActSearchReq) (*model.PaginatedActivities, error)
	FindActByBid(context.Context, string) (model.Activity, error)
}

type ActDao struct {
	DB     *gorm.DB
	effect string
	l      *zap.Logger
}

func NewActDao(db *gorm.DB, cfg *config.Conf, l *logger.LoggerSet) *ActDao {
	return &ActDao{
		DB:     db,
		effect: cfg.Auditor.Effect,
		l:      l.Activity.Named("dao"),
	}
}

func (ad *ActDao) CreateAct(c context.Context, a *model.Activity) error {
	if ad.CheckExist(c, a) {
		ad.l.Warn("tried to create an exist activity", zap.Any("act-bid", a.Bid))
		return errors.New("activity exist")
	} else {
		ad.DB.WithContext(c).Where("student_id = ?", a.StudentID).Delete(model.ActivityDraft{})
		return ad.DB.Create(a).Error
	}
}

func (ad *ActDao) CheckExist(c context.Context, a *model.Activity) bool {
	ret := ad.DB.WithContext(c).Where(&model.Activity{
		Type:       a.Type,
		HolderType: a.HolderType,
		Position:   a.Position,
		IfRegister: a.IfRegister,
	}).Find(&model.Activity{}).RowsAffected
	if ret == 0 {
		return false
	} else {
		return true
	}
}

func (ad *ActDao) CreateDraft(c context.Context, d *model.ActivityDraft) error {
	ad.DB.WithContext(c).Where("student_id = ?", d.StudentID).Delete(&model.ActivityDraft{})
	return ad.DB.Create(d).Error
}

func (ad *ActDao) LoadDraft(c context.Context, s string) (model.ActivityDraft, error) {
	var d model.ActivityDraft

	err := ad.DB.WithContext(c).Preload("Signers").Where("student_id = ?", s).First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ActivityDraft{}, nil
		}
		return model.ActivityDraft{}, err
	}
	return d, nil
}

func (ad *ActDao) DeleteAct(c context.Context, a model.Activity) error {
	ret := ad.DB.WithContext(c).Where(&model.Activity{
		Type:       a.Type,
		HolderType: a.HolderType,
		Position:   a.Position,
		IfRegister: a.IfRegister,
	}).Find(&model.Activity{}).Delete(&model.Activity{}).RowsAffected
	if ret == 0 {
		return errors.New("activity not exist")
	} else {
		return nil
	}
}

func (ad *ActDao) ListAllActs(c context.Context, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit

	err := ad.DB.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}

	err = ad.DB.WithContext(c).Scopes(ad.SetEffect()).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedActivities{
		Total: total,
		Page:  page,
		Limit: limit,
		Acts:  as,
	}, nil
}

func (ad *ActDao) FindActByUser(c context.Context, s string, keyword string, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit

	var err error
	if keyword == "" {
		err = ad.DB.WithContext(c).Preload("Signers").Where("student_id = ? ", s).Limit(limit).Offset(offset).Find(&as).Error
		if err != nil {
			return nil, err
		}
		err = ad.DB.WithContext(c).Where("student_id = ? ", s).Model(&model.Activity{}).Count(&total).Error
		if err != nil {
			return nil, err
		}
	} else {
		err = ad.DB.WithContext(c).Preload("Signers").Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Limit(limit).Offset(offset).Find(&as).Error
		if err != nil {
			return nil, err
		}
		err = ad.DB.WithContext(c).Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Model(&model.Activity{}).Count(&total).Error
		if err != nil {
			return nil, err
		}
	}

	return &model.PaginatedActivities{
		Total: total,
		Page:  page,
		Limit: limit,
		Acts:  as,
	}, nil
}

func (ad *ActDao) FindActByName(c context.Context, n string, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit

	err := ad.DB.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Where("title like ?", fmt.Sprintf("%%%s%%", n)).Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.DB.WithContext(c).Scopes(ad.SetEffect()).Where("title like ?", fmt.Sprintf("%%%s%%", n)).Model(&model.Activity{}).Count(&total).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedActivities{
		Total: total,
		Page:  page,
		Limit: limit,
		Acts:  as,
	}, nil
}

func (ad *ActDao) FindActByDate(c context.Context, d string, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit

	err := ad.DB.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.DB.WithContext(c).Scopes(ad.SetEffect()).Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Model(&model.Activity{}).Count(&total).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedActivities{
		Total: total,
		Page:  page,
		Limit: limit,
		Acts:  as,
	}, nil
}

func (ad *ActDao) FindActByOwnerID(c context.Context, s string, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit

	err := ad.DB.WithContext(c).Preload("Signers").Where("student_id = ?", s).Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.DB.WithContext(c).Where("student_id = ?", s).Model(&model.Activity{}).Count(&total).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedActivities{
		Total: total,
		Page:  page,
		Limit: limit,
		Acts:  as,
	}, nil
}

func (ad *ActDao) FindActBySearches(c context.Context, a *req.ActSearchReq) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	page := a.Page
	limit := a.Limit
	offset := (page - 1) * limit

	q := ad.DB.WithContext(c) // 确保 q 初始化
	if len(a.Type) > 0 {
		q = q.Where("type IN ?", a.Type)
	}
	if len(a.HolderType) > 0 {
		q = q.Where("holder_type IN ?", a.HolderType)
	}
	if len(a.Location) > 0 {
		q = q.Where("position IN ?", a.Location)
	}
	if a.IfRegister != "" {
		q = q.Where("if_register = ?", a.IfRegister)
	}
	if a.DetailTime != "" {
		q = q.Where("start_time <= ? AND end_time >= ?", a.DetailTime, a.DetailTime)
	}

	err := q.Scopes(ad.SetEffect()).Preload("Signers").Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		ad.l.Error("Failed to find activities by searches", zap.Error(err))
		return nil, err
	}

	err = q.Scopes(ad.SetEffect()).Model(&model.Activity{}).Count(&total).Error
	if err != nil {
		ad.l.Error("Failed to count activities by searches", zap.Error(err))
		return nil, err
	}

	return &model.PaginatedActivities{
		Total: total,
		Page:  page,
		Limit: limit,
		Acts:  as,
	}, nil
}

func (ad *ActDao) FindActByBid(c context.Context, bid string) (model.Activity, error) {
	var act model.Activity
	err := ad.DB.WithContext(c).Preload("Signers").Where("bid = ?", bid).First(&act).Error
	if err != nil {
		return model.Activity{}, err
	}
	return act, nil
}

func (ad *ActDao) SetEffect() func(*gorm.DB) *gorm.DB {
	if ad.effect == "slow" {
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("is_checking = ?", "pass")
		}
	} else if ad.effect == "fast" {
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("is_checking != ?", "reject")
		}
	}
	return func(db *gorm.DB) *gorm.DB {
		return db
	}
}

func (ad *ActDao) GetChecking(c context.Context, sid string) ([]model.Activity, error) {
	var acts []model.Activity
	err := ad.DB.WithContext(c).Preload("Signers").Where("student_id = ? AND is_checking = ?", sid, "pending").Find(&acts).Error
	if err != nil {
		ad.l.Error("Failed to get checking activities", zap.Error(err), zap.String("student_id", sid))
		return nil, err
	}
	return acts, nil
}
