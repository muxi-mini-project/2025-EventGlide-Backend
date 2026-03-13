package model

import "time"

type Comment struct {
	Bid       string    `gorm:"unique;primaryKey;not null;type:varchar(255);comment:评论ID;column:bid"`
	CreatedAt time.Time `gorm:"not null;type:datetime;comment:创建时间;column:created_at"`

	StudentID string `gorm:"not null;type:varchar(255);comment:学生ID;column:student_id"`
	Content   string `gorm:"not null;type:text;comment:评论内容;column:content"`
	ParentID  string `gorm:"type:varchar(255);comment:父评论ID;column:parent_id"`
	Position  string `gorm:"not null;type:varchar(255);comment:位置;column:position"`
	Subject   string `gorm:"not null;comment:评论类型;column:subject"` // activity/post/comment
	RootId    string `gorm:"type:varchar(255);comment:根评论ID;column:root_id"`

	LikeNum  int `gorm:"not null;default:0;comment:点赞数;column:like_num"`
	ReplyNum int `gorm:"not null;default:0;comment:回复数;column:reply_num"`
}
