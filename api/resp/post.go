package resp

type CreatePostResp struct {
	Bid         string `json:"bid"`
	StudentID   string `json:"studentId"`
	PublishTime string `json:"publishTime"`

	Title      string   `json:"title"`
	Introduce  string   `json:"introduce"`
	ShowImg    []string `json:"showImg"`
	IsChecking string   `json:"isChecking"`

	UserInfo struct {
		StudentID string `json:"studentId"`
		Avatar    string `json:"avatar"`
		Username  string `json:"username"`
		School    string `json:"school"`
	} `json:"userInfo"`
}

type ListPostsResp struct {
	Bid      string `json:"bid"`
	UserInfo struct {
		StudentID string `json:"studentId"`
		Avatar    string `json:"avatar"`
		Username  string `json:"username"`
		School    string `json:"school"`
	} `json:"userInfo"`
	PublishTime string `json:"publishTime"`

	Introduce string   `json:"introduce"`
	ShowImg   []string `json:"showImg"`
	Title     string   `json:"title"`

	LikeNum    uint `json:"likeNum"`
	CollectNum uint `json:"collectNum"`
	CommentNum uint `json:"commentNum"`

	IsLike     string `json:"isLike"`
	IsCollect  string `json:"isCollect"`
	IsChecking string `json:"isChecking"`
}

type LoadPostDraftResp struct {
	Bid       string   `json:"bid"`
	Title     string   `json:"title"`
	Introduce string   `json:"introduce"`
	ShowImg   []string `json:"showImg"`
	StudentID string   `json:"studentId"`
	CreatedAt string   `json:"createdAt"`
}
