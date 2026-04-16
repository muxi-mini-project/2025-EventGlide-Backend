package resp

type CreateActivityResp struct {
	Bid        string   `json:"bid"`
	Title      string   `json:"title"`
	Introduce  string   `json:"introduce"`
	ShowImg    []string `json:"showImg"`
	Type       string   `json:"type"`
	Position   string   `json:"position"`
	IfRegister string   `json:"ifRegister"`
	IsChecking string   `json:"isChecking"`
	ActiveForm string   `json:"activeForm"`
	Signer     []struct {
		StudentID string `json:"studentId"`
		Name      string `json:"name"`
	} `json:"signer"`
	UserInfo struct {
		StudentID string `json:"studentId"`
		Avatar    string `json:"avatar"`
		Username  string `json:"username"`
		School    string `json:"school"`
	} `json:"userInfo"`
}

type ListActivitiesResp struct {
	UserInfo struct {
		StudentID string `json:"studentId"`
		Avatar    string `json:"avatar"`
		Username  string `json:"username"`
		School    string `json:"school"`
	} `json:"userInfo"`

	DetailTime struct {
		StartTime string `json:"startTime"`
		EndTime   string `json:"endTime"`
	} `json:"detailTime"`

	Title      string   `json:"title"`
	Bid        string   `json:"bid"`
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

type LoadActivitiesDraftResp struct {
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
