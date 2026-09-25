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
	ClaimForUpload(c context.Context, formId int64, now, leaseUntil time.Time) (bool, error)
	ReleaseClaim(c context.Context, formId int64, leaseUntil time.Time) error
	FindPushedPending(c context.Context) ([]model.AuditorForm, error)
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
		Updates(map[string]interface{}{"pushed_at": pushedAt, "claimed_at": nil})
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

// ClaimForUpload 以条件更新原子占用一条未推送表单的上传权（租约）。
// 抢占条件：未推送、且无人持有或租约已过期。返回是否获得占用。
func (a *AuditorRepo) ClaimForUpload(c context.Context, formId int64, now, leaseUntil time.Time) (bool, error) {
	res := a.db.WithContext(c).Model(&model.AuditorForm{}).
		Where("id = ? AND pushed_at IS NULL AND (claimed_at IS NULL OR claimed_at <= ?)", formId, now).
		Update("claimed_at", leaseUntil)
	if res.Error != nil {
		a.l.Error("failed to claim auditor form for upload", zap.Error(res.Error), zap.Int64("formId", formId))
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ReleaseClaim 释放占用，仅当占用值仍为本次持有的租约值时生效，
// 避免误清其它执行者后来建立的新占用。
func (a *AuditorRepo) ReleaseClaim(c context.Context, formId int64, leaseUntil time.Time) error {
	if err := a.db.WithContext(c).Model(&model.AuditorForm{}).
		Where("id = ? AND claimed_at = ?", formId, leaseUntil).
		Update("claimed_at", nil).Error; err != nil {
		a.l.Error("failed to release auditor form claim", zap.Error(err), zap.Int64("formId", formId))
		return err
	}
	return nil
}

// FindPushedPending 返回已成功推送但平台尚未给出结论的表单，
// 供定时回查平台状态（回调可能丢失，导致表单永久停在 pending）。
func (a *AuditorRepo) FindPushedPending(c context.Context) ([]model.AuditorForm, error) {
	var forms []model.AuditorForm
	err := a.db.WithContext(c).
		Where("pushed_at IS NOT NULL AND status = ?", "pending").
		Find(&forms).Error
	if err != nil {
		a.l.Error("failed to find pushed pending auditor forms", zap.Error(err))
		return nil, err
	}
	return forms, nil
}
