package model

import (
	"log"
	"time"

	"gorm.io/gorm"
)

const (
	SubjectActivity = "activity"
	SubjectPost     = "post"
)

type AuditorForm struct {
	Id         int64      `gorm:"primaryKey;type:bigint;column:id"`
	Subject    string     `gorm:"type:varchar(255);not null;uniqueIndex:idx_auditor_form_act_subject,priority:2"`              // 活动 or 帖子
	ActivityId int64      `gorm:"type:bigint;not null;uniqueIndex:idx_auditor_form_act_subject,priority:1;column:activity_id"` // 活动/帖子ID
	Status     string     `gorm:"type:enum('pending','pass','reject');default:'pending';column:status;not null"`               // 表单审核状态 审核是0,1,2
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
			update := tx.Exec(`
				UPDATE activity
				SET is_checking = 'pass'
				WHERE id = ?
				AND is_checking = 'pending_auditor'
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
		}
	}
	return nil
}
