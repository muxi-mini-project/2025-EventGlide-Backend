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
	CreateDraft(context.Context, *gorm.DB, *model.ActivityDraft) error
	LoadDraft(context.Context, string) (model.ActivityDraft, error)
	DeleteAct(context.Context, model.Activity) error
	ListAllActs(context.Context, int, int) (*model.PaginatedActivities, error)
	FindActByUser(context.Context, string, string, int, int) (*model.PaginatedActivities, error)
	FindActByName(context.Context, string, int, int) (*model.PaginatedActivities, error)
	FindActByDate(context.Context, string, int, int) (*model.PaginatedActivities, error)
	FindActByOwnerID(context.Context, string, int, int) (*model.PaginatedActivities, error)
	FindActBySearches(context.Context, *req.ActSearchReq) (*model.PaginatedActivities, error)
	FindActByBid(context.Context, string) (model.Activity, error)
	DeleteActivityDraft(context.Context, *gorm.DB, string) error
	CreateActivity(context.Context, *gorm.DB, *model.Activity) error
	CreateActivitySigners(context.Context, *gorm.DB, []model.ActivitySigner) error
	CreateApprovements(context.Context, *gorm.DB, []model.Approvement) error
	FindDraftsByStudentID(context.Context, *gorm.DB, string, *[]model.ActivityDraft) error
	DeleteSignersByActivityBid(context.Context, *gorm.DB, string) error
	DeleteDraftsByStudentID(context.Context, *gorm.DB, string) error
	BatchCreateSigners(context.Context, *gorm.DB, []model.ActivitySigner) error
}

type ActDao struct {
	db     *gorm.DB
	effect string
	l      *zap.Logger
}

func NewActDao(db *gorm.DB, cfg *config.Conf, l *logger.LoggerSet) *ActDao {
	return &ActDao{
		db:     db,
		effect: cfg.Auditor.Effect,
		l:      l.Activity.Named("dao"),
	}
}

func (ad *ActDao) DB() *gorm.DB {
	return ad.db
}

func (ad *ActDao) DeleteActivityDraft(c context.Context, tx *gorm.DB, studentID string) error {
	return tx.WithContext(c).Where("student_id = ?", studentID).Delete(&model.ActivityDraft{}).Error
}

func (ad *ActDao) CreateActivity(c context.Context, tx *gorm.DB, act *model.Activity) error {
	return tx.WithContext(c).Create(act).Error
}

func (ad *ActDao) CreateActivitySigners(c context.Context, tx *gorm.DB, signers []model.ActivitySigner) error {
	return tx.WithContext(c).Create(&signers).Error
}

func (ad *ActDao) CreateApprovements(c context.Context, tx *gorm.DB, approvements []model.Approvement) error {
	return tx.WithContext(c).Create(&approvements).Error
}

func (ad *ActDao) FindDraftsByStudentID(c context.Context, tx *gorm.DB, studentID string, drafts *[]model.ActivityDraft) error {
	return tx.WithContext(c).Where("student_id = ?", studentID).Find(drafts).Error
}

func (ad *ActDao) DeleteSignersByActivityBid(c context.Context, tx *gorm.DB, activityBid string) error {
	return tx.WithContext(c).Where("activity_bid = ?", activityBid).Delete(&model.ActivitySigner{}).Error
}

func (ad *ActDao) DeleteDraftsByStudentID(c context.Context, tx *gorm.DB, studentID string) error {
	return tx.WithContext(c).Where("student_id = ?", studentID).Delete(&model.ActivityDraft{}).Error
}

func (ad *ActDao) BatchCreateSigners(c context.Context, tx *gorm.DB, signers []model.ActivitySigner) error {
	return tx.WithContext(c).Create(&signers).Error
}

func (ad *ActDao) CreateDraft(c context.Context, tx *gorm.DB, d *model.ActivityDraft) error {
	if err := tx.WithContext(c).Where("student_id = ?", d.StudentID).Delete(&model.ActivityDraft{}).Error; err != nil {
		return err
	}
	return tx.WithContext(c).Create(d).Error
}

func (ad *ActDao) LoadDraft(c context.Context, s string) (model.ActivityDraft, error) {
	var d model.ActivityDraft

	err := ad.db.WithContext(c).Preload("Signers").Where("student_id = ?", s).First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ActivityDraft{}, nil
		}
		return model.ActivityDraft{}, err
	}
	return d, nil
}

func (ad *ActDao) DeleteAct(c context.Context, a model.Activity) error {
	result := ad.db.WithContext(c).Where(&model.Activity{
		Type:       a.Type,
		HolderType: a.HolderType,
		Position:   a.Position,
		IfRegister: a.IfRegister,
	}).Delete(&model.Activity{})
	if result.RowsAffected == 0 {
		return errors.New("activity not exist")
	}
	return nil
}

func (ad *ActDao) ListAllActs(c context.Context, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}

	err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
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
		err = ad.db.WithContext(c).Preload("Signers").Where("student_id = ? ", s).Limit(limit).Offset(offset).Find(&as).Error
		if err != nil {
			return nil, err
		}
		err = ad.db.WithContext(c).Where("student_id = ? ", s).Model(&model.Activity{}).Count(&total).Error
		if err != nil {
			return nil, err
		}
	} else {
		err = ad.db.WithContext(c).Preload("Signers").Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Limit(limit).Offset(offset).Find(&as).Error
		if err != nil {
			return nil, err
		}
		err = ad.db.WithContext(c).Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Model(&model.Activity{}).Count(&total).Error
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

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Where("title like ?", fmt.Sprintf("%%%s%%", n)).Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("title like ?", fmt.Sprintf("%%%s%%", n)).Model(&model.Activity{}).Count(&total).Error
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

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Model(&model.Activity{}).Count(&total).Error
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

	err := ad.db.WithContext(c).Preload("Signers").Where("student_id = ?", s).Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.db.WithContext(c).Where("student_id = ?", s).Model(&model.Activity{}).Count(&total).Error
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

	q := ad.db.WithContext(c)

	listQ := buildActQuery(q, a).Scopes(ad.SetEffect()).Preload("Signers").Limit(limit).Offset(offset)
	if err := listQ.Find(&as).Error; err != nil {
		ad.l.Error("Failed to find activities by searches", zap.Error(err))
		return nil, err
	}

	countQ := buildActQuery(ad.db.WithContext(c), a).Scopes(ad.SetEffect()).Model(&model.Activity{})
	if err := countQ.Count(&total).Error; err != nil {
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
	err := ad.db.WithContext(c).Preload("Signers").Where("bid = ?", bid).First(&act).Error
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

func buildActQuery(db *gorm.DB, a *req.ActSearchReq) *gorm.DB {
	q := db
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
	return q
}

func (ad *ActDao) GetChecking(c context.Context, sid string) ([]model.Activity, error) {
	var acts []model.Activity
	err := ad.db.WithContext(c).Preload("Signers").Where("student_id = ? AND is_checking = ?", sid, "pending").Find(&acts).Error
	if err != nil {
		ad.l.Error("Failed to get checking activities", zap.Error(err), zap.String("student_id", sid))
		return nil, err
	}
	return acts, nil
}
