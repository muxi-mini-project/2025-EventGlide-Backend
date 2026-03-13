package model

import (
	"gorm.io/gorm"
	"log"
	"time"
)

const (
	SubjectActivity = "activity"
	SubjectPost     = "post"
)

type AuditorForm struct {
	Id        uint      `gorm:"primary_key;auto_increment;not null"`
	Subject   string    `gorm:"type:varchar(255);not null"`                                                    // 活动 or 帖子
	Bid       string    `gorm:"type:varchar(255);not null;unique;column:bid"`                                  // 活动/帖子ID
	Status    string    `gorm:"type:enum('pending','pass','reject');default:'pending';column:status;not null"` // 表单审核状态 审核是0,1,2
	FormUrl   string    `gorm:"type:text;column:form_url"`                                                     // 表单的URL地址 // 给活动用的填报表单
	CreatedAt time.Time `gorm:"type:datetime;column:created_at;not null"`                                      // 创建时间
	UpdatedAt time.Time `gorm:"type:datetime;column:updated_at;not null"`                                      // 更新时间
}

func (af *AuditorForm) AfterUpdate(tx *gorm.DB) (err error) {
	if af.Status == StancePass {
		table := af.Subject
		if table == SubjectActivity {
			update := tx.Exec(`
				UPDATE activity
				SET is_checking = 'pass'
				WHERE bid = ?
				AND NOT EXISTS (
					SELECT 1
					FROM approvement
					WHERE bid = ?
					AND stance != 'pass'
				)
			`, af.Bid, af.Bid)
			if update.Error != nil {
				log.Println("auditorform AfterUpdate error when passing activity:", update.Error)
				return update.Error
			}
			if update.RowsAffected > 0 {
				log.Println("auditorform AfterUpdate passed successfully for activity bid:", af.Bid)
				return nil
			}
		} else if table == SubjectPost {
			update := tx.Exec(`
				UPDATE post
				SET is_checking = 'pass'
				WHERE bid = ?
			`, af.Bid)
			if update.Error != nil {
				log.Println("auditorform AfterUpdate error when passing post:", update.Error)
				return update.Error
			}
			if update.RowsAffected > 0 {
				log.Println("auditorform AfterUpdate passed successfully for post bid:", af.Bid)
				return nil
			}
		}
	}

	if af.Status == StanceReject {
		if af.Subject == SubjectActivity {
			update := tx.Exec(`
				UPDATE activity
				SET is_checking = 'reject'
				WHERE bid = ?
			`, af.Bid)
			if update.Error != nil {
				log.Println("auditorform AfterUpdate error when rejecting activity:", update.Error)
				return update.Error
			}
			if update.RowsAffected > 0 {
				log.Println("auditorform AfterUpdate rejected successfully for activity bid:", af.Bid)
				return nil
			}
		} else if af.Subject == SubjectPost {
			update := tx.Exec(`
				UPDATE post
				SET is_checking = 'reject'
				WHERE bid = ?
			`, af.Bid)
			if update.Error != nil {
				log.Println("auditorform AfterUpdate error when rejecting post:", update.Error)
				return update.Error
			}
			if update.RowsAffected > 0 {
				log.Println("auditorform AfterUpdate rejected successfully for post bid:", af.Bid)
				return nil
			}
		}
	}
	return nil
}
