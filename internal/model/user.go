package model

type User struct {
	Id        int    `gorm:"column:id; type: int; not null; primary_key; autoIncrement" json:"id"`
	College   string `gorm:"column:college;type:varchar(255);not null" json:"college"` // 学院
	StudentID string `gorm:"column:student_id;type:varchar(255);not null" json:"studentId"`
	Name      string `gorm:"column:name;type:varchar(255);not null" json:"username"`
	RealName  string `gorm:"column:real_name;type:varchar(255);not null" json:"realName"`
	Avatar    string `gorm:"column:avatar;type:varchar(255);not null" json:"avatar"`
	School    string `gorm:"column:school;type:varchar(255);not null" json:"school"` // 学校
}