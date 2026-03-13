package model

import "time"

type Post struct {
	Id        uint      `gorm:"primaryKey;autoIncrement;comment:主键id;column:id"`
	Bid       string    `gorm:"type: varchar(255);comment:绑定id;column:bid;not null"`
	CreatedAt time.Time `gorm:"type: datetime;comment:创建时间;column:created_at;not null"`

	StudentID string `gorm:"type: varchar(255);comment:学生id;column:student_id;not null" json:"studentId"`
	Title     string `gorm:"type: varchar(255);comment:标题;column:title;not null"`
	Introduce string `gorm:"type: text;comment:帖子描述;column:introduce;not null"`
	ShowImg   string `gorm:"type: text;comment:图片链接;column:show_img"`

	IsChecking string `gorm:"type: enum('pass','reject','checking');default:'checking';comment:审核状态;column:is_checking;not null"`
	LikeNum    uint   `gorm:"type:int unsigned;comment:点赞数;column:like_num;default:0"`
	CollectNum uint   `gorm:"type:int unsigned;comment:收藏数;column:collect_num;default:0"`
	CommentNum uint   `gorm:"type:int unsigned;comment:评论数;column:comment_num;default:0"`
}

type PostDraft struct {
	Bid       string    `gorm:"type: varchar(255);comment:绑定id;column:bid"`
	CreatedAt time.Time `gorm:"type: datetime;comment:创建时间;column:created_at"`

	StudentID string `gorm:"type: varchar(255);comment:学生id;column:student_id"`
	Title     string `gorm:"type: varchar(255);comment:标题;column:title"`
	Introduce string `gorm:"type: text;comment:帖子描述;column:introduce"`
	ShowImg   string `gorm:"type: text;comment:图片链接;column:show_img"`
}
