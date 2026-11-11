package model

import "time"

// 活动审核状态中“审核中”的两种取值（StancePass / StanceReject 见 approvement.go）。
const (
	ActivityPendingSigners = "pending_signers"
	ActivityPendingAuditor = "pending_auditor"
)

// ActivityCheckingVisibleInFast 返回 fast 模式下对外可见的审核状态。
// 用白名单而非“非 reject”：历史 NULL/空值以及以后新增的审核状态都不会被默认放行，
// 与详情侧的可见性判定保持同一集合。返回新切片，调用方无法篡改集合内容。
func ActivityCheckingVisibleInFast() []string {
	return []string{StancePass, ActivityPendingSigners, ActivityPendingAuditor}
}

type Activity struct {
	Id         int64     `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	CreatedAt  time.Time `gorm:"type:datetime;column:created_at; not null"`
	IsChecking string    `gorm:"type:enum('pending_signers','pending_auditor','pass','reject');default:'pending_signers';column:is_checking"` // pending_signers, pending_auditor, pass, reject

	StudentID      string           `gorm:"type:varchar(255);column:student_id;not null;uniqueIndex:idx_activity_unique,priority:1"`
	Title          string           `gorm:"type:varchar(255);column:title;not null;uniqueIndex:idx_activity_unique,priority:2"`
	Revision       int              `gorm:"type:int;column:revision;not null;default:0;uniqueIndex:idx_activity_unique,priority:11"` // 重新送审次数，0 为原始申请；priority 大于默认 10，排在唯一索引末位以保住 (student_id, title, start_time, end_time) 前缀
	ReplacedBy     int64            `gorm:"type:bigint;column:replaced_by;not null;default:0;index"`                                 // 替代它的新申请 id，0 表示未被替代
	Introduce      string           `gorm:"type:text;column:introduce;not null"`
	HolderType     string           `gorm:"type:varchar(255);column:holder_type;not null"`
	OrganizerUnit  string           `gorm:"type:varchar(255);not null;default:'';column:organizer_unit"`
	Position       string           `gorm:"type:varchar(255);column:position;not null"`
	Address        string           `gorm:"type:varchar(255);not null;default:'';column:address"`
	IfRegister     string           `gorm:"type:enum('是','否');column:if_register;not null"`
	RegisterMethod string           `gorm:"type:varchar(255);column:register_method"`
	StartTime      string           `gorm:"type:datetime;column:start_time;not null;uniqueIndex:idx_activity_unique,priority:3"`
	EndTime        string           `gorm:"type:datetime;column:end_time;not null;uniqueIndex:idx_activity_unique,priority:4"`
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
	OrganizerUnit  string           `gorm:"type:varchar(255);column:organizer_unit"`
	Position       string           `gorm:"type:varchar(255);column:position"`
	Address        string           `gorm:"type:varchar(255);column:address"`
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
	Id         int64           `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	ActivityId int64           `gorm:"type:bigint;index"`
	StudentID  string          `gorm:"type:varchar(255);not null"`
	Name       EncryptedString `gorm:"type:varchar(255);not null"`
}

// 图片宽高解析三态：pending 待解析 / ok 已解析 / failed 永久失败。
// 宽高列不再用负值表达状态：failed 行宽高为 0，前端按未知尺寸兜底；
// 重试 failed 行：UPDATE image SET size_status = 0 WHERE size_status = 2。
const (
	ImageSizePending = 0
	ImageSizeOK      = 1
	ImageSizeFailed  = 2
)

// ImageSizeMaxEdge 宽高合理上限：超过视为异常值（前端传错或图片头损坏），
// 按未知尺寸处理，防止超 INT 上限的脏值写库失败或外露。
const ImageSizeMaxEdge = 20000

type Image struct {
	Id         int64  `gorm:"primaryKey;type:bigint;comment:主键id;column:id"`
	OwnerId    int64  `gorm:"type:bigint;comment:所有者ID;column:owner_id;not null;index"`
	OwnerType  string `gorm:"type:varchar(50);comment:所有者类型;column:owner_type;not null;index"`
	Url        string `gorm:"type:varchar(500);comment:图片链接;column:url;not null;index"`
	Width      int    `gorm:"type:int;comment:图片宽度;column:width;default:0"`
	Height     int    `gorm:"type:int;comment:图片高度;column:height;default:0"`
	SizeStatus int    `gorm:"type:tinyint;comment:宽高解析状态 0待解析 1已解析 2失败;column:size_status;default:0;index"`
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
