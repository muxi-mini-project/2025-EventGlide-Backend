package req

type ActSearchReq struct {
	Type       []string `json:"type,omitempty"`
	HolderType []string `json:"holderType,omitempty"`
	Location   []string `json:"location,omitempty"`
	IfRegister string   `json:"ifRegister,omitempty"`
	DetailTime struct {
		StartTime string `json:"startTime,omitempty"`
		EndTime   string `json:"endTime,omitempty"`
	} `json:"detailTime,omitempty"`
}

type CreateActReq struct {
	Title     string   `json:"title" validate:"required"`
	Introduce string   `json:"introduce" validate:"required"`
	ShowImg   []string `json:"showImg"`

	LabelForm struct {
		HolderType     string `json:"holderType" validate:"required"`
		Position       string `json:"position" validate:"required"`
		IfRegister     string `json:"ifRegister" validate:"required,oneof=是 否"`
		RegisterMethod string `json:"registerMethod"`
		StartTime      string `json:"startTime" validate:"required,ltcsfield=EndTime"`
		ActiveForm     string `json:"activeForm" validate:"required"`
		EndTime        string `json:"endTime" validate:"required,gtcsfield=StartTime"`
		Type           string `json:"type" validate:"required"`

		Signer []struct {
			StudentID string `json:"studentId" validate:"len=10"`
			Name      string `json:"name"`
		} `json:"signer" validate:"required_if=HolderType 个人,dive"`
	} `json:"labelform"`
}

type CreateActDraftReq struct {
	Title     string   `json:"title"`
	Introduce string   `json:"introduce"`
	ShowImg   []string `json:"showImg"`

	LabelForm struct {
		HolderType     string `json:"holderType"`
		Position       string `json:"position"`
		IfRegister     string `json:"ifRegister"`
		RegisterMethod string `json:"registerMethod"`
		StartTime      string `json:"startTime"`
		ActiveForm     string `json:"activeForm"`
		EndTime        string `json:"endTime"`
		Type           string `json:"type"`

		Signer []struct {
			StudentID string `json:"studentId" validate:"len=10"`
			Name      string `json:"name"`
		} `json:"signer"`
	} `json:"labelform"`
}

type FindActByNameReq struct {
	Name string `json:"name" validate:"required"`
}

type FindActByDateReq struct {
	Date string `json:"date" validate:"required"` // 02-01
}
