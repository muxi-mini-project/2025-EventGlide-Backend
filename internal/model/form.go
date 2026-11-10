package model

import (
	"log"
	"time"

	"gorm.io/gorm"
)

const (
	SubjectActivity = "activity"
	SubjectPost     = "post"

	// AuditorReasonMaxLen 与 AuditorForm.Reason 的列宽 varchar(512) 保持一致。
	// MySQL 的 varchar 按字符计数，超长在严格模式下会报 1406 使整条更新失败（状态也写不进去），
	// 故写库前需按 rune 截断。
	AuditorReasonMaxLen = 512
)

type AuditorForm struct {
	Id         int64      `gorm:"primaryKey;type:bigint;column:id"`
	Subject    string     `gorm:"type:varchar(255);not null;uniqueIndex:idx_auditor_form_act_subject,priority:2"`              // 活动 or 帖子
	ActivityId int64      `gorm:"type:bigint;not null;uniqueIndex:idx_auditor_form_act_subject,priority:1;column:activity_id"` // 活动/帖子ID
	Status     string     `gorm:"type:enum('pending','pass','reject');default:'pending';column:status;not null"`               // 表单审核状态 审核是0,1,2
	Reason     string     `gorm:"type:varchar(512);column:reason;not null;default:''"`                                         // 审核不通过原因；仅 reject 时写入，来自回调 Data.Msg
	FormUrl    string     `gorm:"type:text;column:form_url"`                                                                   // 表单的URL地址 // 给活动用的填报表单
	PushedAt   *time.Time `gorm:"type:datetime;column:pushed_at"`                                                              // 成功送审时间；NULL 表示尚未推送
	ClaimedAt  *time.Time `gorm:"type:datetime;column:claimed_at"`                                                             // 上传占用（租约）；NULL 表示无人持有，供多实例互斥
	CreatedAt  time.Time  `gorm:"type:datetime;column:created_at;not null"`                                                    // 创建时间
	UpdatedAt  time.Time  `gorm:"type:datetime;column:updated_at;not null"`                                                    // 更新时间
}

func (af *AuditorForm) AfterUpdate(tx *gorm.DB) (err error) {
	// 帖子不审核：审核结论不再回写帖子状态。否则部署时已在审核中的帖子被回填为 pass 后，
	// 平台迟到的 reject 回调会把它重新置为隐藏，与"帖子不审核"矛盾。
	// 非活动表单一律不同步活动：ParkAsPending 之类的条件更新不加载整行，hook 收到的是零值表单
	// （Subject 为空），若按 activity_id=0 去改活动会误伤无关数据。
	if af.Subject != SubjectActivity {
		return nil
	}

	if af.Status == StancePass {
		// 平台在"通过方向"上是审核结论的权威源：条目先被判不通过、之后又被改判为通过时，
		// 已置为 reject 的活动必须跟着放行，否则会出现"平台显示通过、用户侧仍被拒"的错位。
		// replaced_by = 0：被新申请替代的旧活动即使平台改判通过也不重新上架。
		return af.syncActivityChecking(tx, "pass", `
			UPDATE activity
			SET is_checking = 'pass'
			WHERE id = ?
			AND is_checking IN ('pending_auditor','reject')
			AND replaced_by = 0
			AND NOT EXISTS (
				SELECT 1
				FROM approvement
				WHERE activity_id = ?
				AND stance != 'pass'
			)
		`, af.ActivityId, af.ActivityId)
	}

	if af.Status == StanceReject {
		// 已发布（pass）的活动被改判不通过时也要下架，否则平台显示不通过、用户侧仍可见。
		return af.syncActivityChecking(tx, "reject", `
			UPDATE activity
			SET is_checking = 'reject'
			WHERE id = ?
			AND is_checking IN ('pending_auditor','pass')
		`, af.ActivityId)
	}

	if af.Status == StancePending {
		// 平台把条目重置为未审核：已出结论的活动退回审核中，结论由后续回调/对账重新落库。
		// 已过期的活动不在对账范围内（见 FindPushedPending），被替代的旧活动也不再参与审核流转。
		return af.syncActivityChecking(tx, "pending_auditor", `
			UPDATE activity
			SET is_checking = 'pending_auditor'
			WHERE id = ?
			AND is_checking IN ('pass','reject')
			AND replaced_by = 0
		`, af.ActivityId)
	}

	return nil
}

// syncActivityChecking 执行一条活动状态迁移，并在命中 0 行时以活动当前状态判断是否需要告警：
// 活动已处于 target 表示同结论此前已落库（例如退回待审后又由对账写回），属正常路径静默跳过；
// 只有结论确实没落到活动上才告警，避免对账每轮刷假告警。
func (af *AuditorForm) syncActivityChecking(tx *gorm.DB, target, query string, args ...interface{}) error {
	update := tx.Exec(query, args...)
	if update.Error != nil {
		log.Printf("auditorform AfterUpdate error when setting activity is_checking=%s (form=%d activity=%d): %v", target, af.Id, af.ActivityId, update.Error)
		return update.Error
	}
	if update.RowsAffected > 0 {
		log.Printf("auditorform AfterUpdate: activity %d is_checking -> %s (form=%d)", af.ActivityId, target, af.Id)
		return nil
	}
	var cur string
	if qerr := tx.Model(&Activity{}).Select("COALESCE(is_checking, '')").Where("id = ?", af.ActivityId).Scan(&cur).Error; qerr != nil {
		log.Printf("auditorform AfterUpdate WARN: form(id=%d) status=%s subject=%s activity=%d saved, but failed to load activity status: %v", af.Id, af.Status, af.Subject, af.ActivityId, qerr)
		return nil
	}
	if cur == target {
		return nil
	}
	log.Printf("auditorform AfterUpdate WARN: form(id=%d) status=%s subject=%s activity=%d saved, but activity is_checking=%q (expected %s)", af.Id, af.Status, af.Subject, af.ActivityId, cur, target)
	return nil
}
