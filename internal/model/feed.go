package model

import "time"

type Feed struct {
	Id        int64     `gorm:"column:id; type:bigint; primaryKey; autoIncrement; comment:主键" json:"id"`                                            // 主键
	TargetBid string    `gorm:"column:target_bid; type:varchar(40); not null; comment:目标id; uniqueIndex:idx_feed_business_unique" json:"target_id"` // 目标id
	RootID    string    `gorm:"column:root_id; type:varchar(40); comment:评论归属根对象ID" json:"root_id"`
	RootType  string    `gorm:"column:root_type; type:varchar(20); comment:评论归属根对象类型" json:"root_type"`
	Object    string    `gorm:"column:object; type:varchar(20); not null; comment:目标主题; uniqueIndex:idx_feed_business_unique" json:"object"`        // 活动还是帖子
	StudentId string    `gorm:"column:student_id; type:varchar(10); not null; comment:学生id; uniqueIndex:idx_feed_business_unique" json:"studentid"` // 发起者
	Receiver  string    `gorm:"column:receiver; type:varchar(10); not null; comment:接收者; uniqueIndex:idx_feed_business_unique" json:"receiver"`     // 接收者
	CreatedAt time.Time `gorm:"column:created_at; type:datetime; not null; comment:创建时间"`
	Action    string    `gorm:"column:action; type:varchar(30); not null; comment:行为; uniqueIndex:idx_feed_business_unique"`
	Status    string    `gorm:"column:status; type:varchar(20); not null; comment:状态; default:'未读'"`
}
