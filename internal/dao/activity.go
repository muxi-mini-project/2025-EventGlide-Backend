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
	FindActListForStudent(context.Context, string, string, string, int, int) (*model.PaginatedActivities, error)
	FindActBySearches(context.Context, *req.ActSearchReq) (*model.PaginatedActivities, error)
	FindActById(context.Context, int64) (model.Activity, error)
	DeleteActivityDraft(context.Context, *gorm.DB, string) error
	CreateActivity(context.Context, *gorm.DB, *model.Activity) error
	CreateActivitySigners(context.Context, *gorm.DB, []model.ActivitySigner) error
	CreateApprovements(context.Context, *gorm.DB, []model.Approvement) error
	FindDraftsByStudentID(context.Context, *gorm.DB, string, *[]model.ActivityDraft) error
	DeleteSignersByActivityId(context.Context, *gorm.DB, int64) error
	DeleteDraftsByStudentID(context.Context, *gorm.DB, string) error
	BatchCreateSigners(context.Context, *gorm.DB, []model.ActivitySigner) error
	FindRejectedAct(context.Context, string, string, string, string) (model.Activity, error) // studentID, title, startTime, endTime
	FindPendingAuditorActivities(context.Context) ([]model.Activity, error)
	MarkActivityReplaced(context.Context, *gorm.DB, int64, int64) (bool, error) // srcID, newID
	ExistsApprovement(context.Context, int64, string) (bool, error)             // activityID, studentID
}

type ActDao struct {
	db     *gorm.DB
	effect string
	l      *zap.Logger
}

func NewActDao(db *gorm.DB, cfg *config.Conf, l *logger.LoggerSet) *ActDao {
	return &ActDao{
		db:     db,
		effect: cfg.Auditor.EffectMode(),
		l:      l.Activity.Named("dao"),
	}
}

func (ad *ActDao) DB() *gorm.DB {
	return ad.db
}

func (ad *ActDao) DeleteActivityDraft(c context.Context, tx *gorm.DB, studentID string) error {
	return ad.deleteActivityDraftsByStudentID(c, tx, studentID)
}

func (ad *ActDao) CreateActivity(c context.Context, tx *gorm.DB, act *model.Activity) error {
	images := act.Images
	act.Images = nil

	if err := tx.WithContext(c).Create(act).Error; err != nil {
		return err
	}

	if len(images) > 0 {
		for i := range images {
			images[i].OwnerId = act.Id
		}
		if err := tx.WithContext(c).Create(&images).Error; err != nil {
			return err
		}
	}
	return nil
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

func (ad *ActDao) DeleteSignersByActivityId(c context.Context, tx *gorm.DB, activityId int64) error {
	return tx.WithContext(c).Where("activity_id = ?", activityId).Delete(&model.ActivitySigner{}).Error
}

func (ad *ActDao) DeleteDraftsByStudentID(c context.Context, tx *gorm.DB, studentID string) error {
	return ad.deleteActivityDraftsByStudentID(c, tx, studentID)
}

// deleteActivityDraftsByStudentID 删除该学生全部活动草稿，并一并清理其关联图片。
// 草稿「先删后建」每次保存都会重插 image 行，旧行须显式清理，否则残留孤儿行。
func (ad *ActDao) deleteActivityDraftsByStudentID(c context.Context, tx *gorm.DB, studentID string) error {
	var ids []int64
	if err := tx.WithContext(c).Model(&model.ActivityDraft{}).Where("student_id = ?", studentID).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if err := deleteImagesByOwner(c, tx, "activity_draft", ids); err != nil {
		return err
	}
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

	err := ad.db.WithContext(c).Preload("Signers").Preload("Images").Where("student_id = ?", s).First(&d).Error
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

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
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
		err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("student_id = ? ", s).Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
		if err != nil {
			return nil, err
		}
		err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("student_id = ? ", s).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
		if err != nil {
			return nil, err
		}
	} else {
		err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
		if err != nil {
			return nil, err
		}
		err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("student_id = ? and title like ?", s, fmt.Sprintf("%%%s%%", keyword)).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
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

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("title like ?", fmt.Sprintf("%%%s%%", n)).Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("title like ?", fmt.Sprintf("%%%s%%", n)).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
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

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("start_time like ?", fmt.Sprintf("%%%s%%", d)).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
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

	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("student_id = ?", s).Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset).Find(&as).Error
	if err != nil {
		return nil, err
	}
	err = ad.db.WithContext(c).Scopes(ad.SetEffect()).Where("student_id = ?", s).Where("end_time > ?", time.Now()).Model(&model.Activity{}).Count(&total).Error
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

// FindActListForStudent 返回学生本人视角的活动列表：默认只含自己创建的活动，
// role 可扩展到"我签署的"（approvement 子查询）；scope 控制时间范围与已结束活动的审核状态。
// 不套 SetEffect——本人应能看到自己的审核中与被拒活动。
// scope/role 需已由上层用 req.ActScope*/req.ActRole* 归一化，未知值按 active/creator 处理。
// 列表与 count 通过同一份条件闭包构造，保证口径一致。
func (ad *ActDao) FindActListForStudent(c context.Context, s string, scope string, role string, page, limit int) (*model.PaginatedActivities, error) {
	var as []model.Activity
	var total int64
	offset := (page - 1) * limit
	now := time.Now()
	base := ad.db.WithContext(c)

	conds := func(db *gorm.DB) *gorm.DB {
		db = db.Model(&model.Activity{})

		switch role {
		case req.ActRoleParticipant:
			db = db.Where("id IN (?)", base.Model(&model.Approvement{}).Select("activity_id").Where("student_id = ?", s))
		case req.ActRoleAll:
			db = db.Where("(student_id = ? OR id IN (?))", s,
				base.Model(&model.Approvement{}).Select("activity_id").Where("student_id = ?", s))
		default:
			db = db.Where("student_id = ?", s)
		}

		switch scope {
		case req.ActScopeEnded:
			// 已结束：只保留已通过的活动，pending/reject 不再出现在本人列表。
			db = db.Where("end_time <= ?", now).Where("is_checking = ?", model.StancePass)
		case req.ActScopeAll:
			db = db.Where("(end_time > ? OR is_checking = ?)", now, model.StancePass)
		default:
			db = db.Where("end_time > ?", now)
		}
		return db
	}

	// 次级排序键保证同一时间点的多行在翻页时顺序稳定，避免重复或遗漏。
	order := "start_time ASC, id ASC"
	switch scope {
	case req.ActScopeEnded:
		order = "end_time DESC, id DESC"
	case req.ActScopeAll:
		order = "start_time DESC, id DESC"
	}

	if err := conds(base).Preload("Signers").Preload("Images").
		Order(order).Limit(limit).Offset(offset).Find(&as).Error; err != nil {
		return nil, err
	}
	if err := conds(base).Count(&total).Error; err != nil {
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

	listQ := buildActQuery(q, a).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("end_time > ?", time.Now()).Order("start_time ASC").Limit(limit).Offset(offset)
	if err := listQ.Find(&as).Error; err != nil {
		ad.l.Error("Failed to find activities by searches", zap.Error(err))
		return nil, err
	}

	countQ := buildActQuery(ad.db.WithContext(c), a).Scopes(ad.SetEffect()).Where("end_time > ?", time.Now()).Model(&model.Activity{})
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

func (ad *ActDao) FindActById(c context.Context, id int64) (model.Activity, error) {
	var act model.Activity
	err := ad.db.WithContext(c).Preload("Signers").Preload("Images").Where("id = ?", id).First(&act).Error
	if err != nil {
		return model.Activity{}, err
	}
	return act, nil
}

// FindActAuditBrief 只取可见性判断需要的列，不预加载签署人与图片，
// 供评论列表等高频读路径使用。
func (ad *ActDao) FindActAuditBrief(c context.Context, id int64) (model.Activity, error) {
	var act model.Activity
	err := ad.db.WithContext(c).Select("id", "student_id", "is_checking").Where("id = ?", id).First(&act).Error
	if err != nil {
		return model.Activity{}, err
	}
	return act, nil
}

// SetEffect 按审核模式过滤列表：fast 先发后审，只挡被驳回的活动；
// 其余（含未配置）按 slow 先审后发，只放已通过的活动。
func (ad *ActDao) SetEffect() func(*gorm.DB) *gorm.DB {
	if ad.effect == config.EffectFast {
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("is_checking != ?", "reject")
		}
	}
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_checking = ?", "pass")
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

// GetChecking 返回本人仍处于审核中的活动（/user/checking）。
// 已过期的活动不再返回：它们不会再送审、也不会再有结论，继续显示"审核中"没有意义。
func (ad *ActDao) GetChecking(c context.Context, sid string) ([]model.Activity, error) {
	var acts []model.Activity
	err := ad.db.WithContext(c).Preload("Signers").Preload("Images").
		Where("student_id = ? AND is_checking IN ?", sid, []string{"pending_signers", "pending_auditor"}).
		Where("end_time > ?", time.Now()).
		Find(&acts).Error
	if err != nil {
		ad.l.Error("Failed to get checking activities", zap.Error(err), zap.String("student_id", sid))
		return nil, err
	}
	return acts, nil
}

func (ad *ActDao) FindRejectedAct(c context.Context, studentID, title, startTime, endTime string) (model.Activity, error) {
	var act model.Activity
	err := ad.db.WithContext(c).Where("student_id = ? AND title = ? AND start_time = ? AND end_time = ? AND is_checking = ?", studentID, title, startTime, endTime, "reject").Preload("Signers").Preload("Images").First(&act).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Activity{}, nil
		}
		ad.l.Error("Failed to find rejected activity", zap.Error(err))
		return model.Activity{}, err
	}
	return act, nil
}

// FindPendingAuditorActivities 返回待送审的活动。已过期的活动不再送审：
// 它们在所有列表都不可见，送进平台只会占用审核队列且永远不会对用户生效。
func (ad *ActDao) FindPendingAuditorActivities(c context.Context) ([]model.Activity, error) {
	var acts []model.Activity
	err := ad.db.WithContext(c).Where("is_checking = 'pending_auditor'").Where("end_time > ?", time.Now()).Preload("Signers").Preload("Images").Find(&acts).Error
	if err != nil {
		return nil, err
	}
	return acts, nil
}

func (ad *ActDao) FindActsByIds(c context.Context, ids []int64) ([]model.Activity, error) {
	if len(ids) == 0 {
		return []model.Activity{}, nil
	}
	var acts []model.Activity
	err := ad.db.WithContext(c).Scopes(ad.SetEffect()).Preload("Signers").Preload("Images").Where("id IN ?", ids).Find(&acts).Error
	return acts, err
}

// MarkActivityReplaced 把源活动的 replaced_by 指向新活动。
// 条件更新同时校验 replaced_by = 0 与 is_checking = reject：并发重复提交只有一个能成功，
// 且在读取源活动与本次更新之间被平台改判（reject -> pass）时不会替代一条已公开的活动。
// 返回 false 表示条件未命中（已被替代或状态已变化），调用方应回滚事务。
func (ad *ActDao) MarkActivityReplaced(c context.Context, tx *gorm.DB, srcID, newID int64) (bool, error) {
	res := tx.WithContext(c).Model(&model.Activity{}).
		Where("id = ? AND replaced_by = 0 AND is_checking = ?", srcID, model.StanceReject).
		Update("replaced_by", newID)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ExistsApprovement 判断某学号是否为该活动的签署人（approvement 只记录除申请人外的签署人）。
func (ad *ActDao) ExistsApprovement(c context.Context, activityID int64, studentID string) (bool, error) {
	var count int64
	err := ad.db.WithContext(c).Model(&model.Approvement{}).
		Where("activity_id = ? AND student_id = ?", activityID, studentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
