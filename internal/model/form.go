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
	if af.Subject == SubjectPost {
		return nil
	}

	if af.Status == StancePass {
		table := af.Subject
		if table == SubjectActivity {
			// 平台在"通过方向"上是审核结论的权威源：条目先被判不通过、之后又被改判为通过时，
			// 已置为 reject 的活动必须跟着放行，否则会出现"平台显示通过、用户侧仍被拒"的错位。
			// reject 分支不做对称扩展（不把已 pass 的活动改判为 reject 下架）：下架属于产品本次划定
			// 范围之外的场景，留待后续评估，与上面的放行不冲突，勿顺手补成对称条件。
			update := tx.Exec(`
				UPDATE activity
				SET is_checking = 'pass'
				WHERE id = ?
				AND is_checking IN ('pending_auditor','reject')
				AND NOT EXISTS (
					SELECT 1
					FROM approvement
					WHERE activity_id = ?
					AND stance != 'pass'
				)
			`, af.ActivityId, af.ActivityId)
			if update.Error != nil {
				log.Println("auditorform AfterUpdate error when passing activity:", update.Error)
				return update.Error
			}
			if update.RowsAffected > 0 {
				log.Println("auditorform AfterUpdate passed successfully for activity:", af.ActivityId)
				return nil
			}
			// 结论已落库但活动未变更：活动可能已是 pass/reject，或送审人守卫不满足。
			// 平台存在 AI 与人工两套回调，同一结论的重复回调属正常，故仅 Warn 便于排查。
			log.Printf("auditorform AfterUpdate WARN: form(id=%d) status=%s subject=%s activity=%d saved, but activity unchanged (already pass/reject, or signer guard not met); duplicate callbacks of the same conclusion are expected", af.Id, af.Status, af.Subject, af.ActivityId)
		}
	}

	if af.Status == StanceReject {
		if af.Subject == SubjectActivity {
			update := tx.Exec(`
				UPDATE activity
				SET is_checking = 'reject'
				WHERE id = ?
				AND is_checking = 'pending_auditor'
			`, af.ActivityId)
			if update.Error != nil {
				log.Println("auditorform AfterUpdate error when rejecting activity:", update.Error)
				return update.Error
			}
			if update.RowsAffected > 0 {
				log.Println("auditorform AfterUpdate rejected successfully for activity:", af.ActivityId)
				return nil
			}
			// 结论已落库但活动未变更：活动已不在 pending_auditor（reject 分支没有送审人守卫）。
			// 平台存在 AI 与人工两套回调，同一结论的重复回调属正常，故仅 Warn 便于排查。
			log.Printf("auditorform AfterUpdate WARN: form(id=%d) status=%s subject=%s activity=%d saved, but activity not in pending_auditor; duplicate callbacks of the same conclusion are expected", af.Id, af.Status, af.Subject, af.ActivityId)
		}
	}
	return nil
}
