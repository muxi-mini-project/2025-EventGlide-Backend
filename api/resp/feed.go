package resp

type BriefFeedResp struct {
	LikeAndCollect int `json:"likeAndCollect"`
	CommentAndAt   int `json:"commentAndAt"`
	Total          int `json:"total"`
}

type FeedResp struct {
	Likes       []FeedLikeResp       `json:"likes,omitempty"`
	Ats         []FeedAtResp         `json:"ats,omitempty"`
	Comments    []FeedCommentResp    `json:"comments,omitempty"`
	Collects    []FeedCollectResp    `json:"collects,omitempty"`
	Invitations []FeedInvitationResp `json:"invitations,omitempty"`
}

type UserInfo struct {
	StudentID string `json:"studentId"`
	Avatar    string `json:"avatar"`
	Username  string `json:"username"`
}
type FeedLikeResp struct {
	Userinfo UserInfo `json:"userInfo"`

	Id          int64  `json:"id"`
	Message     string `json:"message"`
	PublishedAt string `json:"publishedAt"`
	TargetBid   string `json:"targetBid"`
	FirstPic    string `json:"firstPic,omitempty"`
	Status      string `json:"status"`
}

type FeedCommentResp struct {
	Userinfo UserInfo `json:"userInfo"`

	Id          int64  `json:"id"`
	Message     string `json:"message"`
	PublishedAt string `json:"publishedAt"`
	TargetBid   string `json:"targetBid"`
	FirstPic    string `json:"firstPic,omitempty"`
	Status      string `json:"status"`
}

type FeedAtResp struct {
	Userinfo UserInfo `json:"userInfo"`

	Id          int64  `json:"id"`
	Message     string `json:"message"`
	PublishedAt string `json:"publishedAt"`
	TargetBid   string `json:"targetBid"`
	FirstPic    string `json:"firstPic,omitempty"`
	Status      string `json:"status"`
}

type FeedCollectResp struct {
	Userinfo UserInfo `json:"userInfo"`

	Id          int64  `json:"id"`
	Message     string `json:"message"`
	PublishedAt string `json:"publishedAt"`
	FirstPic    string `json:"firstPic,omitempty"`
	TargetBid   string `json:"targetBid"`
	Status      string `json:"status"`
}

type FeedInvitationResp struct {
	Userinfo UserInfo `json:"userInfo"`

	Id          int64  `json:"id"`
	Message     string `json:"message"`
	PublishedAt string `json:"publishedAt"`
	TargetBid   string `json:"targetBid"`
	FirstPic    string `json:"firstPic,omitempty"`
	Status      string `json:"status"`
}
