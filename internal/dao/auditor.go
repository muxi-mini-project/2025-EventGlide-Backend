package dao

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuditorRepository interface {
	Insert(c context.Context, bid string, formUrl string, sub string) (*model.AuditorForm, error)
	Update(c context.Context, formId uint, status string) error
	Get(c context.Context, bid string) (model.AuditorForm, error)
	IsRejected(c context.Context, bid string) (bool, error)
}
type AuditorRepo struct {
	db *gorm.DB

	l *zap.Logger
}

func NewAuditorRepo(db *gorm.DB, l *zap.Logger) AuditorRepository {
	return &AuditorRepo{
		db: db,
		l:  l,
	}
}

func (a *AuditorRepo) Insert(c context.Context, bid string, formUrl string, sub string) (*model.AuditorForm, error) {
	form := model.AuditorForm{
		Bid:     bid,
		FormUrl: formUrl,
		Subject: sub,
	}
	if err := a.db.WithContext(c).Create(&form).Error; err != nil {
		return nil, err
	}

	return &form, nil
}

func (a *AuditorRepo) Update(c context.Context, formId uint, status string) error {
	var form model.AuditorForm
	if err := a.db.WithContext(c).Model(&model.AuditorForm{}).Where("id = ?", formId).First(&form).Error; err != nil {
		a.l.Error("auditor form not found", zap.Error(err))
		return err
	}
	form.Status = status
	if err := a.db.WithContext(c).Save(&form).Error; err != nil {
		a.l.Error("failed to update auditor form", zap.Error(err))
		return err
	}
	return nil
}

func (a *AuditorRepo) Get(c context.Context, bid string) (model.AuditorForm, error) {
	var form model.AuditorForm
	err := a.db.WithContext(c).Where("bid = ?", bid).First(&form).Error
	return form, err
}

func (a *AuditorRepo) IsRejected(c context.Context, bid string) (bool, error) {
	var form model.AuditorForm
	err := a.db.WithContext(c).Where("bid = ? and status = ?", bid, "rejected").First(&form).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil // Not rejected
	}
	return true, err // Either found or another error occurred
}
