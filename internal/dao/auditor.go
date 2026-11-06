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
	"gorm.io/gorm/clause"
)

type AuditorRepository interface {
	Insert(c context.Context, activityId int64, formUrl string, sub string) (*model.AuditorForm, error)
	Update(c context.Context, formId int64, status string, reason string) error
	FindReasonsByActivityIDs(c context.Context, ids []int64) (map[int64]string, error)
	FindByActivity(c context.Context, activityId int64, sub string) (model.AuditorForm, error)
	MarkPushed(c context.Context, formId int64, pushedAt time.Time) error
	ClaimForUpload(c context.Context, formId int64, now, leaseUntil time.Time) (bool, error)
	ReleaseClaim(c context.Context, formId int64, leaseUntil time.Time) error
	FindPushedPending(c context.Context, afterID int64, limit int) ([]model.AuditorForm, error)
	UpdateIfPending(c context.Context, formId int64, status string) error
	RevertRejectToPending(c context.Context, formId int64) error
	FindByID(c context.Context, formId int64) (model.AuditorForm, error)
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

func (a *AuditorRepo) Update(c context.Context, formId int64, status string, reason string) error {
	var form model.AuditorForm
	if err := a.db.WithContext(c).Model(&model.AuditorForm{}).Where("id = ?", formId).First(&form).Error; err != nil {
		// 回调查不到本地表单行：帖子撤销删除后迟到回调和"hook_id 与本地行对不上"都落在这里。
		// 均返回成功（避免平台无休止重试），但升到 Warn 以便观测异常回调，而非静默 Info。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.l.Warn("auditor form not found on update, treated as success", zap.Int64("formId", formId), zap.String("status", status))
			return nil
		}
		a.l.Error("auditor form not found", zap.Error(err))
		return err
	}
	prevStatus := form.Status
	form.Status = status
	// 原因只服务当前结论：非 reject 时清空，避免条目重置/对账把上一轮的旧原因当成当前原因展示；
	// 批量查询已按 status='reject' 过滤，清空不损失展示。
	// reject 回调可能不带原因（人工审核常见），空串不覆盖已有原因，避免把第一次写入的原因冲掉。
	if status == model.StanceReject {
		if reason != "" {
			form.Reason = reason
		}
	} else {
		form.Reason = ""
	}
	// 只写结论相关列：回调与上传/撤销并发时，读全行再 Save 会用旧快照覆盖 pushed_at/claimed_at，
	// 使已推送表单被当作未推送或占用被误清。
	if err := a.db.WithContext(c).Model(&form).Select("Status", "Reason").Updates(&form).Error; err != nil {
		a.l.Error("failed to update auditor form", zap.Error(err))
		return err
	}
	// 结论发生迁移时记录旧状态：回调没有顺序/版本号，reject 被改判为 pass 需要能从日志还原是谁盖了谁。
	if prevStatus != "" && prevStatus != status {
		if prevStatus == model.StanceReject && status == model.StancePass {
			a.l.Warn("auditor form transitioned reject -> pass by callback",
				zap.Int64("formId", formId), zap.Int64("activityId", form.ActivityId),
				zap.String("from", prevStatus), zap.String("to", status))
		} else {
			a.l.Info("auditor form status changed by callback",
				zap.Int64("formId", formId), zap.Int64("activityId", form.ActivityId),
				zap.String("from", prevStatus), zap.String("to", status))
		}
	}
	return nil
}

// FindReasonsByActivityIDs 批量返回活动的审核原因（activity_id -> reason，空串也返回），
// 供列表接口一次取全，避免逐条查询。ids 为空时不发 SQL。
func (a *AuditorRepo) FindReasonsByActivityIDs(c context.Context, ids []int64) (map[int64]string, error) {
	reasons := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return reasons, nil
	}

	var forms []model.AuditorForm
	if err := a.db.WithContext(c).
		Select("activity_id", "reason").
		Where("subject = ? AND status = ? AND activity_id IN ?", model.SubjectActivity, model.StanceReject, ids).
		Find(&forms).Error; err != nil {
		a.l.Error("failed to find auditor reasons", zap.Error(err))
		return nil, err
	}
	for _, form := range forms {
		reasons[form.ActivityId] = form.Reason
	}
	return reasons, nil
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

// FindPushedPending 按 id 游标分批返回"已推送但平台仍未给结论"的活动表单，
// 供定时对账分批回查（回调可能丢失，导致表单永久停在 pending）。
// 以 id 升序、取 id > afterID 的前 limit 条；调用方处理完后推进游标，取完回绕。
// 排除帖子表单：帖子已不审核，其历史表单在平台上可能已不存在，
// 继续回查只会每轮刷告警且永远不收敛。
// 同时排除所属活动已过期的表单：这类活动在所有列表都不可见，结论对用户没有意义。
func (a *AuditorRepo) FindPushedPending(c context.Context, afterID int64, limit int) ([]model.AuditorForm, error) {
	var forms []model.AuditorForm
	err := a.db.WithContext(c).
		Where("pushed_at IS NOT NULL AND status = ? AND subject <> ? AND id > ?", "pending", model.SubjectPost, afterID).
		Where("EXISTS (SELECT 1 FROM activity a WHERE a.id = auditor_form.activity_id AND a.end_time > ?)", time.Now()).
		Order("id ASC").
		Limit(limit).
		Find(&forms).Error
	if err != nil {
		a.l.Error("failed to find pushed pending auditor forms", zap.Error(err))
		return nil, err
	}
	return forms, nil
}

// UpdateIfPending 仅在表单当前仍为 pending 时更新状态，避免对账用平台过期结果覆盖
// 回调已写入的终态（pass/reject）。条件未命中（已被回调更新）时视为无需处理，返回 nil。
// 在事务内对行加锁，使"读当前状态 -> 写回"对同一行的并发回调写入原子，消除 TOCTOU。
// 平台结论与现值一致时不写库；状态确有变化时仍走 Save，以触发 AuditorForm.AfterUpdate 推进活动。
func (a *AuditorRepo) UpdateIfPending(c context.Context, formId int64, status string) error {
	return a.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		var form model.AuditorForm
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", formId).First(&form).Error; err != nil {
			// 表单已被撤销删除（帖子已删）：对账视为幂等成功，避免与撤销并发时对已删行报错刷日志。
			if errors.Is(err, gorm.ErrRecordNotFound) {
				a.l.Info("auditor form already gone on reconcile update, treated as success", zap.Int64("formId", formId))
				return nil
			}
			a.l.Error("auditor form not found", zap.Error(err), zap.Int64("formId", formId))
			return err
		}
		if form.Status != "pending" {
			a.l.Info("auditor form no longer pending, skip reconcile update",
				zap.Int64("formId", formId), zap.String("status", form.Status))
			return nil
		}
		// 平台结论与现值一致：跳过写库，避免每轮对账反复刷新 updated_at。
		if form.Status == status {
			return nil
		}
		form.Status = status
		// 原因只服务 reject 结论：写回非 reject 时清空。否则"驳回(有原因) -> 回查无法确认退回
		// pending（保留原因） -> 对账写回 pass"后旧原因会残留，被后续不带原因的 reject 回调沿用。
		if status != model.StanceReject {
			form.Reason = ""
		}
		if err := tx.Save(&form).Error; err != nil {
			a.l.Error("failed to update auditor form", zap.Error(err), zap.Int64("formId", formId))
			return err
		}
		return nil
	})
}

// RevertRejectToPending 仅在表单当前仍为 reject 时退回 pending。
// 改判回查无法确认平台结论时用它代替无条件的 Update：条件更新不会覆盖并发路径已写入的
// 终态（例如已确认的 pass），也不会因非 reject 写入而清空驳回原因。
// 条件未命中（状态已被回调/对账推进）视为幂等成功。
func (a *AuditorRepo) RevertRejectToPending(c context.Context, formId int64) error {
	form := model.AuditorForm{Id: formId, Status: "pending"}
	res := a.db.WithContext(c).
		Model(&model.AuditorForm{}).
		Where("id = ? AND status = ?", formId, model.StanceReject).
		Select("Status").
		Updates(&form)
	if res.Error != nil {
		a.l.Error("failed to revert auditor form to pending", zap.Error(res.Error), zap.Int64("formId", formId))
		return res.Error
	}
	if res.RowsAffected == 0 {
		a.l.Info("auditor form no longer reject, skip revert to pending", zap.Int64("formId", formId))
	}
	return nil
}

// FindByID 按主键回查表单最新状态（撤销前取，避免用到过期快照）。
func (a *AuditorRepo) FindByID(c context.Context, formId int64) (model.AuditorForm, error) {
	var form model.AuditorForm
	err := a.db.WithContext(c).Where("id = ?", formId).First(&form).Error
	return form, err
}
