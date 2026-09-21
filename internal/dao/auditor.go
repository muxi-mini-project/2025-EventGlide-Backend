package dao

import (
	"context"
	"errors"
	"time"

	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuditorRepository interface {
	Insert(c context.Context, activityId int64, formUrl string, sub string) (*model.AuditorForm, error)
	Update(c context.Context, formId int64, status string) error
	Get(c context.Context, activityId int64) (model.AuditorForm, error)
	IsRejected(c context.Context, activityId int64) (bool, error)
	FindByActivity(c context.Context, activityId int64, sub string) (model.AuditorForm, error)
	MarkPushed(c context.Context, formId int64, pushedAt time.Time) error
}
type AuditorRepo struct {
	db *gorm.DB

	l *zap.Logger
}

func NewAuditorRepo(db *gorm.DB, l *logger.LoggerSet) AuditorRepository {
	return &AuditorRepo{
		db: db,
		l:  l.Auditor.Named("dao"),
	}
}

func (a *AuditorRepo) Insert(c context.Context, activityId int64, formUrl string, sub string) (*model.AuditorForm, error) {
	form := model.AuditorForm{
		Id:         tools.MustGenerateID(),
		ActivityId: activityId,
		FormUrl:    formUrl,
		Subject:    sub,
	}
	if err := a.db.WithContext(c).Create(&form).Error; err != nil {
		return nil, err
	}

	return &form, nil
}

func (a *AuditorRepo) Update(c context.Context, formId int64, status string) error {
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

func (a *AuditorRepo) Get(c context.Context, activityId int64) (model.AuditorForm, error) {
	var form model.AuditorForm
	err := a.db.WithContext(c).Where("activity_id = ?", activityId).First(&form).Error
	return form, err
}

func (a *AuditorRepo) IsRejected(c context.Context, activityId int64) (bool, error) {
	var form model.AuditorForm
	err := a.db.WithContext(c).Where("activity_id = ? and status = ?", activityId, "reject").First(&form).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil // Not rejected
	}
	return true, err // Either found or another error occurred
}

func (a *AuditorRepo) FindByActivity(c context.Context, activityId int64, sub string) (model.AuditorForm, error) {
	var form model.AuditorForm
	err := a.db.WithContext(c).
		Where("activity_id = ? AND subject = ?", activityId, sub).
		First(&form).Error
	return form, err
}

func (a *AuditorRepo) MarkPushed(c context.Context, formId int64, pushedAt time.Time) error {
	res := a.db.WithContext(c).Model(&model.AuditorForm{}).
		Where("id = ? AND pushed_at IS NULL", formId).
		Update("pushed_at", pushedAt)
	if res.Error != nil {
		a.l.Error("failed to mark auditor form pushed", zap.Error(res.Error), zap.Int64("formId", formId))
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 条件未命中：表单不存在，或已被其它执行者标记（幂等成功）。
		var form model.AuditorForm
		if err := a.db.WithContext(c).Select("pushed_at").Where("id = ?", formId).First(&form).Error; err != nil {
			a.l.Error("failed to reload auditor form after mark", zap.Error(err), zap.Int64("formId", formId))
			return err
		}
		if form.PushedAt == nil {
			a.l.Error("auditor form mark pushed matched no row but pushed_at still null", zap.Int64("formId", formId))
			return errors.New("auditor form not marked pushed")
		}
		a.l.Info("auditor form already pushed, treated as success", zap.Int64("formId", formId))
	}
	return nil
}
