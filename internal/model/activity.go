package model

import "time"

type Activity struct {
	Bid        string    `gorm:"type:varchar(255);primary_key;not null;unique;column:bid"`
	CreatedAt  time.Time `gorm:"type:datetime;column:created_at; not null"`
	IsChecking string    `gorm:"type:enum('pass','pending','reject');default:'pending';column:is_checking"` // pending or pass or reject // 是否显示

	StudentID      string `gorm:"type:varchar(255);column:student_id;not null"`
	Title          string `gorm:"type:varchar(255);column:title;not null;uniqueIndex:idx_activity_unique"`
	Introduce      string `gorm:"type:text;column:introduce;not null"`
	ShowImg        string `gorm:"type:text;column:show_img"`
	HolderType     string `gorm:"type:varchar(255);column:holder_type;not null"`
	Position       string `gorm:"type:varchar(255);column:position;not null"`
	IfRegister     string `gorm:"type:enum('是','否');column:if_register;not null"`
	RegisterMethod string `gorm:"type:varchar(255);column:register_method"`
	StartTime      string `gorm:"type:datetime;column:start_time;not null;uniqueIndex:idx_activity_unique"`
	EndTime        string `gorm:"type:datetime;column:end_time;not null;uniqueIndex:idx_activity_unique"`
	Type           string `gorm:"type:varchar(255);column:type;not null"`
	ActiveForm     string `gorm:"type:varchar(255);column:active_form"` // 表单url // 通过条件2
	Signer         string `gorm:"type:text;column:signer;not null"`     // 报名人 >= 5的 []slice // 通过条件1

	LikeNum    uint `gorm:"type:int unsigned;column:like_num;default:0"`
	CollectNum uint `gorm:"type:int unsigned;column:collect_num;default:0"`
	CommentNum uint `gorm:"type:int unsigned;column:comment_num;default:0"`
}

type ActivityDraft struct {
	Bid       string    `gorm:"type:varchar(255);primary_key;not null;unique;column:bid"`
	CreatedAt time.Time `gorm:"type:datetime;column:created_at;not null" `

	StudentID      string `gorm:"type:varchar(255);column:student_id"`
	Title          string `gorm:"type:varchar(255);column:title"`
	Introduce      string `gorm:"type:text;column:introduce"`
	ShowImg        string `gorm:"type:text;column:show_img"`
	HolderType     string `gorm:"type:varchar(255);column:holder_type"`
	Position       string `gorm:"type:varchar(255);column:position"`
	IfRegister     string `gorm:"type:varchar(32);column:if_register"`
	RegisterMethod string `gorm:"type:varchar(255);column:register_method"`
	StartTime      string `gorm:"type:varchar(255);column:start_time"`
	EndTime        string `gorm:"type:varchar(255);column:end_time"`
	Type           string `gorm:"type:varchar(255);column:type"`
	ActiveForm     string `gorm:"type:varchar(255);column:active_form"`
	Signer         string `gorm:"type:text;column:signer"`
}

type Signer struct {
	StudentID string
	Name      string
}

type UserBrief struct {
	StudentID string
	Name      string
	Avatar    string
	School    string
}

type ActivityDetail struct {
	Activity  Activity
	Author    UserBrief
	IsLike    bool
	IsCollect bool
}
