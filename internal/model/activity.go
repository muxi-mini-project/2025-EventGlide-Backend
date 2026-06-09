package model

import "time"

type Activity struct {
	Id         int64     `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	CreatedAt  time.Time `gorm:"type:datetime;column:created_at; not null"`
	IsChecking string    `gorm:"type:enum('pending_signers','pending_auditor','pass','reject');default:'pending_signers';column:is_checking"` // pending_signers, pending_auditor, pass, reject

	StudentID      string           `gorm:"type:varchar(255);column:student_id;not null;uniqueIndex:idx_activity_unique"`
	Title          string           `gorm:"type:varchar(255);column:title;not null;uniqueIndex:idx_activity_unique"`
	Introduce      string           `gorm:"type:text;column:introduce;not null"`
	HolderType     string           `gorm:"type:varchar(255);column:holder_type;not null"`
	Position       string           `gorm:"type:varchar(255);column:position;not null"`
	IfRegister     string           `gorm:"type:enum('是','否');column:if_register;not null"`
	RegisterMethod string           `gorm:"type:varchar(255);column:register_method"`
	StartTime      string           `gorm:"type:datetime;column:start_time;not null;uniqueIndex:idx_activity_unique"`
	EndTime        string           `gorm:"type:datetime;column:end_time;not null;uniqueIndex:idx_activity_unique"`
	Type           string           `gorm:"type:varchar(255);column:type;not null"`
	ActiveForm     string           `gorm:"type:varchar(255);column:active_form"`
	Signers        []ActivitySigner `gorm:"foreignKey:ActivityId;references:Id;constraint:false"`
	LikeNum        uint             `gorm:"type:int unsigned;column:like_num;default:0"`
	CollectNum     uint             `gorm:"type:int unsigned;column:collect_num;default:0"`
	CommentNum     uint             `gorm:"type:int unsigned;column:comment_num;default:0"`
	Images         []Image          `gorm:"foreignKey:OwnerId;references:Id;constraint:false"`

	SignerCount int `gorm:"type:int;column:signer_count;default:0"`
	SignedCount int `gorm:"type:int;column:signed_count;default:0"`
}

type ActivityDraft struct {
	Id        int64     `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	CreatedAt time.Time `gorm:"type:datetime;column:created_at;not null" `

	StudentID      string           `gorm:"type:varchar(255);column:student_id"`
	Title          string           `gorm:"type:varchar(255);column:title"`
	Introduce      string           `gorm:"type:text;column:introduce"`
	HolderType     string           `gorm:"type:varchar(255);column:holder_type"`
	Position       string           `gorm:"type:varchar(255);column:position"`
	IfRegister     string           `gorm:"type:varchar(32);column:if_register"`
	RegisterMethod string           `gorm:"type:varchar(255);column:register_method"`
	StartTime      string           `gorm:"type:varchar(255);column:start_time"`
	EndTime        string           `gorm:"type:varchar(255);column:end_time"`
	Type           string           `gorm:"type:varchar(255);column:type"`
	ActiveForm     string           `gorm:"type:varchar(255);column:active_form"`
	Signers        []ActivitySigner `gorm:"foreignKey:ActivityId;references:Id;constraint:false"`
	Images         []Image          `gorm:"foreignKey:OwnerId;references:Id;constraint:false"`
}

type ActivitySigner struct {
	Id         int64  `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	ActivityId int64  `gorm:"type:bigint;index"`
	StudentID  string `gorm:"type:varchar(255);not null"`
	Name       string `gorm:"type:varchar(255);not null"`
}

type Image struct {
	Id        int64  `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	OwnerId   int64  `gorm:"type:bigint;comment:所有者ID;column:owner_id;not null;index"`
	OwnerType string `gorm:"type:varchar(50);comment:所有者类型;column:owner_type;not null;index"`
	Url       string `gorm:"type:text;comment:图片链接;column:url;not null"`
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
	Images    []Image
	Signers   []ActivitySigner
	IsLike    bool
	IsCollect bool
}

type PaginatedActivities struct {
	Total int64
	Page  int
	Limit int
	Acts  []Activity
}
