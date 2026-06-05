package model

import (
	"gorm.io/gorm"
	"log"
	"time"
)

const (
	StancePass   = "pass"
	StanceReject = "reject"
)

type Approvement struct {
	Id          int64    `gorm:"primaryKey;type:bigint;column:id"`
	ActivityId  int64    `gorm:"type:bigint;column:activity_id;not null"`
	StudentId   string    `gorm:"type:varchar(255);not null;column:student_id"`
	StudentName string    `gorm:"type:varchar(255);not null;column:student_name"`
	Stance      string    `gorm:"type:enum('pass','reject','pending');default:'pending';column:stance;not null"`
	UpdatedAt   time.Time `gorm:"type:datetime;column:updated_at;not null"`
	CreatedAt   time.Time `gorm:"type:datetime;column:created_at;not null"`
}

func (a *Approvement) AfterUpdate(tx *gorm.DB) (err error) {
	if a.Stance == StancePass {
		passUpdate := tx.Exec(`
			UPDATE activity
			SET is_checking = 'pass'
			WHERE id = ?
			AND NOT EXISTS (
				SELECT 1
				FROM approvement
				WHERE activity_id = ?
				AND stance != 'pass'
			)
			AND EXISTS (
				SELECT 1
				FROM auditor_form
				WHERE activity_id = ?
				AND status = 'pass'
			)
		`, a.ActivityId, a.ActivityId, a.ActivityId)
		if passUpdate.Error != nil {
			log.Println("approvement AfterUpdate error when passing:", passUpdate.Error)
			return passUpdate.Error
		}
		if passUpdate.RowsAffected > 0 {
			log.Println("approvement AfterUpdate passed successfully for activity:", a.ActivityId)
			return nil
		}
	} else if a.Stance == StanceReject {
		rejectUpdate := tx.Exec(`
			UPDATE activity
			SET is_checking = 'reject'
			WHERE id = ?
		`, a.ActivityId)
		if rejectUpdate.Error != nil {
			log.Println("approvement AfterUpdate error when rejecting:", rejectUpdate.Error)
			return rejectUpdate.Error
		}
		if rejectUpdate.RowsAffected > 0 {
			log.Println("approvement AfterUpdate rejected successfully for activity:", a.ActivityId)
			return nil
		}
	}
	return nil
}