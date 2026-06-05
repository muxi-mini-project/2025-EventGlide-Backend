package resp

type CreateActivityResp struct {
	Id         int64   `json:"id"`
	Title      string   `json:"title"`
	Introduce  string   `json:"introduce"`
	ShowImg    []string `json:"showImg"`
	Type       string   `json:"type"`
	Position   string   `json:"position"`
	IfRegister string   `json:"ifRegister"`
	IsChecking string   `json:"isChecking"`
	ActiveForm string   `json:"activeForm"`
	Signer     []Signer `json:"signer"`
	UserInfo   UserInfo `json:"userInfo"`
}

type ListActivitiesResp struct {
	UserInfo UserInfo `json:"userInfo"`

	DetailTime DetailTime `json:"detailTime"`

	Title      string   `json:"title"`
	Id         int64   `json:"id"`
	Introduce  string   `json:"introduce"`
	Position   string   `json:"position"`
	Type       string   `json:"type"`
	HolderType string   `json:"holderType"`
	IfRegister string   `json:"ifRegister"`
	ShowImg    []string `json:"showImg"`
	IsChecking string   `json:"isChecking"`

	LikeNum    uint `json:"likeNum"`
	CollectNum uint `json:"collectNum"`
	CommentNum uint `json:"commentNum"`

	IsLike    string `json:"isLike"`
	IsCollect string `json:"isCollect"`
}

type PaginatedListActivitiesResp struct {
	Total   int64                  `json:"total"`
	Page    int                    `json:"page"`
	Limit   int                    `json:"limit"`
	Details []ListActivitiesResp   `json:"details"`
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
	Position       string `json:"position"`
	IfRegister     string `json:"ifRegister"`
	RegisterMethod string `json:"registerMethod"`
	StartTime      string `json:"startTime"`
	ActiveForm     string `json:"activeForm"`
	EndTime        string `json:"endTime"`
	Type           string `json:"type"`

	Signer []Signer `json:"signer" validate:"required_if=HolderType 个人,unique=StudentID,dive"`
}