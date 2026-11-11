package resp

import "github.com/raiki02/EG/pkg/utils"

type CreateActivityResp struct {
	Id            utils.SnowflakeID `json:"id"`
	Title         string            `json:"title"`
	Introduce     string            `json:"introduce"`
	ShowImg       []string          `json:"showImg"`
	Type          string            `json:"type"`
	OrganizerUnit string            `json:"organizerUnit"`
	Position      string            `json:"position"`
	Address       string            `json:"address"`
	IfRegister    string            `json:"ifRegister"`
	IsChecking    string            `json:"isChecking"`
	ActiveForm    string            `json:"activeForm"`
	Signer        []Signer          `json:"signer"`
	// Revision 为重新送审版本号（原始申请为 0）；ReplacedBy 为替代它的新申请 id，
	// 序列化成字符串，"0" 表示未被替代（不是空串或 null）。
	Revision   int               `json:"revision"`
	ReplacedBy utils.SnowflakeID `json:"replacedBy"`
	UserInfo   UserInfo          `json:"userInfo"`
}

type ListActivitiesResp struct {
	UserInfo UserInfo `json:"userInfo"`

	DetailTime DetailTime `json:"detailTime"`

	Title         string            `json:"title"`
	Id            utils.SnowflakeID `json:"id"`
	Introduce     string            `json:"introduce"`
	OrganizerUnit string            `json:"organizerUnit"`
	Position      string            `json:"position"`
	Address       string            `json:"address"`
	Type          string            `json:"type"`
	HolderType    string            `json:"holderType"`
	IfRegister    string            `json:"ifRegister"`
	ShowImg       []string          `json:"showImg"`
	IsChecking    string            `json:"isChecking"`
	RejectReason  string            `json:"rejectReason"`
	// 以下字段按权限返回：revision/replacedBy 在活动详情与创建者本人的列表（/act/own）中返回；
	// activeForm/signer 只在详情中返回。公开列表共用该结构，这些字段均为空值。
	Revision   int               `json:"revision"`
	ReplacedBy utils.SnowflakeID `json:"replacedBy"`
	ActiveForm string            `json:"activeForm"`
	Signer     []Signer          `json:"signer"`

	LikeNum    uint `json:"likeNum"`
	CollectNum uint `json:"collectNum"`
	CommentNum uint `json:"commentNum"`

	IsLike    string `json:"isLike"`
	IsCollect string `json:"isCollect"`
}

type PaginatedListActivitiesResp struct {
	Total   int64                `json:"total"`
	Page    int                  `json:"page"`
	Limit   int                  `json:"limit"`
	Details []ListActivitiesResp `json:"details"`
}

type LoadActivitiesDraftResp struct {
	Title     string   `json:"title"`
	Introduce string   `json:"introduce"`
	ShowImg   []string `json:"showImg"`

	LabelForm LabelForm `json:"labelform"`
}

type Signer struct {
	StudentID string `json:"studentId" validate:"len=10"`
	Name      string `json:"name"`
}

type DetailTime struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type LabelForm struct {
	HolderType     string `json:"holderType"`
	OrganizerUnit  string `json:"organizerUnit"`
	Position       string `json:"position"`
	Address        string `json:"address"`
	IfRegister     string `json:"ifRegister"`
	RegisterMethod string `json:"registerMethod"`
	StartTime      string `json:"startTime"`
	ActiveForm     string `json:"activeForm"`
	EndTime        string `json:"endTime"`
	Type           string `json:"type"`

	Signer []Signer `json:"signer" validate:"required_if=HolderType 个人,unique=StudentID,dive"`
}
