package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ActDaoHdl interface {
	CreateAct(context.Context, *model.Activity) error
	CreateDraft(context.Context, *model.ActivityDraft) error
	DeleteAct(context.Context, model.Activity) error
	LoadDraft(context.Context, string, string) (*model.ActivityDraft, error)
	FindActByName(context.Context, string) ([]model.Activity, error)
	FindActByDate(context.Context, string) ([]model.Activity, error)
	FindActByOwnerID(context.Context, string) ([]model.Activity, error)
	CheckExist(context.Context, *model.Activity) bool
	ListAllActs(context.Context) ([]model.Activity, error)
	FindActBySearches(context.Context, *req.ActSearchReq) ([]model.Activity, error)
}

type ActDao struct {
	db     *gorm.DB
	effect string
	l      *zap.Logger
}

func NewActDao(db *gorm.DB, l *zap.Logger, cfg *config.Conf) *ActDao {
	return &ActDao{
		db:     db,
		effect: cfg.Auditor.Effect,
		l:      l.Named("activity/dao"),
	}
}

func (ad *ActDao) CreateAct(c context.Context, a *model.Activity) error {
	if ad.CheckExist(c, a) {
		ad.l.Warn("tried to create an exist activity", zap.Any("act-bid", a.Bid))
		return errors.New("activity exist")
	} else {
		ad.db.WithContext(c).Where("student_id = ?", a.StudentID).Delete(model.ActivityDraft{})
		return ad.db.Create(a).Error
	}
}

func (ad *ActDao) CreateDraft(c context.Context, d *model.ActivityDraft) error {
	ad.db.WithContext(c).Where("student_id = ?", d.StudentID).Delete(&model.ActivityDraft{})
	return ad.db.Create(d).Error
}

func (ad *ActDao) LoadDraft(c context.Context, s string) (model.ActivityDraft, error) {
	var d model.ActivityDraft
	err := ad.db.WithContext(c).Where("student_id = ?", s).Find(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return d, nil
		}
		return model.ActivityDraft{}, err
	}
	return d, nil
}

// TODO: 换成按页展示，每页返回固定个数活动
func (ad *ActDao) FindActByUser(c context.Context, s string, keyword string) ([]model.Activity, error) {
	var as []model.Activity
	if keyword == "" {
		err := ad.db.WithContext(c).Where("student_id = ? ", s).Find(&as).Error
		if err != nil {
			return nil, err
		}
		return as, nil
	} else {
		err := ad.db.WithContext(c).Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Find(&as).Error
		if err != nil {
			return nil, err
		}
		return as, nil
	}
}

func (ad *ActDao) FindActByName(c context.Context, n string) ([]model.Activity, error) {
	var as []model.Activity
	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("title like ?", fmt.Sprintf("%%%s%%", n)).Find(&as).Error
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (ad *ActDao) FindActByDate(c context.Context, d string) ([]model.Activity, error) {
	var as []model.Activity
	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Find(&as).Error
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (ad *ActDao) CheckExist(c context.Context, a *model.Activity) bool {
	ret := ad.db.WithContext(c).Where(&model.Activity{
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

func (ad *ActDao) DeleteAct(c context.Context, a model.Activity) error {
	ret := ad.db.WithContext(c).Where(&model.Activity{
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

func (ad *ActDao) FindActBySearches(c context.Context, a *req.ActSearchReq) ([]model.Activity, error) {
	var as []model.Activity
	q := ad.db.WithContext(c) // 确保 q 初始化
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
	if a.DetailTime.StartTime != "" && a.DetailTime.EndTime != "" {
		q = q.Where("start_time >= ? AND end_time <= ?", a.DetailTime.StartTime, a.DetailTime.EndTime)
	}

	err := q.Scopes(ad.SetEffect()).Find(&as).Error
	if err != nil {
		ad.l.Error("Failed to find activities by searches", zap.Error(err))
	}

	return as, err
}

func (ad *ActDao) FindActByOwnerID(c context.Context, s string) ([]model.Activity, error) {
	var as []model.Activity
	err := ad.db.WithContext(c).Where("student_id = ?", s).Find(&as).Error
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (ad *ActDao) ListAllActs(c context.Context) ([]model.Activity, error) {
	var as []model.Activity

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Find(&as).Error
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (ad *ActDao) FindActByBid(c context.Context, bid string) (model.Activity, error) {
	var act model.Activity
	err := ad.db.WithContext(c).Where("bid = ?", bid).First(&act).Error
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
	err := ad.db.WithContext(c).Where("student_id = ? AND is_checking = ?", sid, "pending").Find(&acts).Error
	if err != nil {
		ad.l.Error("Failed to get checking activities", zap.Error(err), zap.String("student_id", sid))
		return nil, err
	}
	return acts, nil
}
