package req

type ActSearchReq struct {
	Type       []string `json:"type,omitempty"`
	HolderType []string `json:"holderType,omitempty"`
	Location   []string `json:"location,omitempty"`
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
	Date string `json:"date" validate:"required"` // 02-01
	Page int    `json:"page,omitempty"`
	Limit int   `json:"limit,omitempty"`
}

type FindActByBidReq struct {
	Id string `json:"id" validate:"required" form:"id" uri:"id"`
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

type FindActByOwnerIDReq struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type Signer struct {
	StudentID string `json:"studentId" validate:"len=10"`
	Name      string `json:"name"`
}

type CreateActLabel struct {
	HolderType     string `json:"holderType" validate:"required"`
	Position       string `json:"position" validate:"required"`
	IfRegister     string `json:"ifRegister" validate:"required,oneof=是 否"`
	RegisterMethod string `json:"registerMethod"`
	StartTime      string `json:"startTime" validate:"required,ltcsfield=EndTime"`
	ActiveForm     string `json:"activeForm" validate:"required,required_unless=HolderType 个人"`
	EndTime        string `json:"endTime" validate:"required,gtcsfield=StartTime"`
	Type           string `json:"type" validate:"required"`

	Signer []Signer `json:"signer" validate:"unique=StudentID,dive"`
}

type CreateDraftLabel struct {
	HolderType     string `json:"holderType"`
	Position       string `json:"position"`
	IfRegister     string `json:"ifRegister"`
	RegisterMethod string `json:"registerMethod"`
	StartTime      string `json:"startTime"`
	ActiveForm     string `json:"activeForm"`
	EndTime        string `json:"endTime"`
	Type           string `json:"type"`

	Signer []Signer `json:"signer"`
}
