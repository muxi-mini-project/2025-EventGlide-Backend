package req

import "github.com/raiki02/EG/pkg/utils"

type ActSearchReq struct {
	Type       []string `json:"type,omitempty"`
	HolderType []string `json:"holderType,omitempty"`
	Location   []string `json:"position,omitempty"`
	IfRegister string   `json:"ifRegister,omitempty"`
	DetailTime string   `json:"detailTime,omitempty"`
	Page       int      `json:"page,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

type CreateActReq struct {
	Title     string   `json:"title" validate:"required"`
	Introduce string   `json:"introduce" validate:"required"`
	ShowImg   []string `json:"showImg"`

	LabelForm CreateActLabel `json:"labelform"`
}

type CreateActDraftReq struct {
	Title     string   `json:"title"`
	Introduce string   `json:"introduce"`
	ShowImg   []string `json:"showImg"`

	LabelForm CreateDraftLabel `json:"labelform"`
}

type FindActByNameReq struct {
	Name  string `json:"name" validate:"required"`
	Page  int    `json:"page,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type FindActByDateReq struct {
	Date  string `json:"date" validate:"required"` // 02-01
	Page  int    `json:"page,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type FindActByIdReq struct {
	Id utils.SnowflakeID `json:"id" validate:"required" form:"-" uri:"id" swaggertype:"string"`
}

// ResubmitActReq 重新送审请求：路径参数带源活动 id，请求体复用创建活动的完整内容。
type ResubmitActReq struct {
	Id utils.SnowflakeID `json:"-" form:"-" uri:"id" validate:"required"`
	CreateActReq
}

type FindActByUserIDReq struct {
	UserID string `json:"userId" validate:"required" form:"userId" uri:"userId"`
	Page   int    `json:"page,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type ListAllActsReq struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// 活动列表 scope / role 的合法取值。
const (
	ActScopeActive = "active" // 进行中（默认）
	ActScopeEnded  = "ended"  // 已结束且已通过
	ActScopeAll    = "all"    // 进行中或已通过

	ActRoleCreator     = "creator"     // 我创建的（默认）
	ActRoleParticipant = "participant" // 我签署的
	ActRoleAll         = "all"         // 我创建的 + 我签署的
)

type FindActByOwnerIDReq struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	// Scope 时间范围：active(默认，进行中) | ended(已结束且已通过) | all(进行中或已通过)。
	// 空值与非法值按 active 处理。
	// all 不是 active 的超集：ended/all 排除已被新申请替代的旧申请，active 保留它们供审核页使用。
	Scope string `json:"scope"`
	// Role 归属范围：creator(默认，我创建的) | participant(我签署的) | all(两者)。
	// 空值与非法值按 creator 处理。
	Role string `json:"role"`
}

type FindActByStudentIDReq struct {
	// 学号只从路径参数取：body 里即使带 studentId 也忽略，避免覆盖路径值。
	StudentID string `json:"-" form:"studentId" uri:"studentId"`
	Page      int    `json:"page"`
	Limit     int    `json:"limit"`
	// Scope 仅本人视角且显式传参时生效：active(默认，进行中) | ended(已结束且已通过) | all(进行中或已通过)。
	// 传了 scope 或 role 任一参数即切到本人全状态口径，都不传时保持旧的“已通过 + 未结束”行为。
	// 空值与非法值按 active 处理；查看他人时忽略。
	// all 不是 active 的超集：ended/all 排除已被新申请替代的旧申请，active 保留它们供审核页使用。
	Scope string `json:"scope"`
	// Role 仅本人视角且显式传参时生效：creator(默认，我创建的) | participant(我签署的) | all(两者)。
	// 传了 scope 或 role 任一参数即切到本人全状态口径。
	// 空值与非法值按 creator 处理；查看他人时忽略。
	Role string `json:"role"`
}

type Signer struct {
	StudentID string `json:"studentId" validate:"len=10"`
	Name      string `json:"name" validate:"max=30"`
}

type CreateActLabel struct {
	HolderType     string `json:"holderType" validate:"required"`
	OrganizerUnit  string `json:"organizerUnit" validate:"required"`
	Position       string `json:"position" validate:"required"`
	Address        string `json:"address" validate:"required"`
	IfRegister     string `json:"ifRegister" validate:"required,oneof=是 否"`
	RegisterMethod string `json:"registerMethod"`
	StartTime      string `json:"startTime" validate:"required,ltcsfield=EndTime"`
	ActiveForm     string `json:"activeForm" validate:"required"`
	EndTime        string `json:"endTime" validate:"required,gtcsfield=StartTime"`
	Type           string `json:"type" validate:"required"`

	Signer []Signer `json:"signer" validate:"unique=StudentID,dive"`
}

type CreateDraftLabel struct {
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

	Signer []Signer `json:"signer"`
}
