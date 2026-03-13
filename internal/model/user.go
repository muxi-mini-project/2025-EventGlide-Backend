package model

type User struct {
	Id          int    `gorm:"column:id; type: int; not null; primary_key; autoIncrement" json:"id"`
	College     string `gorm:"column:college;type:varchar(255);not null" json:"college"` // 学院
	StudentID   string `gorm:"column:student_id;type:varchar(255);not null" json:"studentId"`
	Name        string `gorm:"column:name;type:varchar(255);not null" json:"username"`
	Avatar      string `gorm:"column:avatar;type:varchar(255);not null" json:"avatar"`
	School      string `gorm:"column:school;type:varchar(255);not null" json:"school"` // 学校
	CollectAct  string `gorm:"column:collect_act;type:text"`
	LikeAct     string `gorm:"column:like_act;type:text"`
	CollectPost string `gorm:"column:collect_post;type:text"`
	LikePost    string `gorm:"column:like_post;type:text"`
	LikeComment string `gorm:"column:like_comment;type:text"`
}
