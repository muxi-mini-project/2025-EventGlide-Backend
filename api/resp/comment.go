package resp

type CommentResp struct {
	Id int64 `json:"id"`

	Creator Creator `json:"creator"`

	CommentedTime string `json:"commentedTime"`
	CommentedPos  string `json:"commentedPos"`
	Content       string `json:"content"`
	LikeNum       int    `json:"likeNum"`
	ReplyNum      int    `json:"replyNum"`
	IsLike        string `json:"isLike"`
	ParentID      int64 `json:"parentId"`
	RootID        int64 `json:"rootId"`

	Reply []ReplyResp `json:"reply"`
}

type ReplyResp struct {
	Id int64 `json:"id"`

	ReplyCreator ReplyCreator `json:"replyCreator"`

	ReplyContent string `json:"replyContent"`
	ReplyTime    string `json:"replyTime"`
	ReplyPos     string `json:"replyPos"`

	ParentID       int64 `json:"parentId"`
	RootID         int64 `json:"rootId"`
	ParentUserName string `json:"parentUserName"`

	IsLike   string `json:"isLike"`
	LikeNum  int    `json:"likeNum"`
	ReplyNum int    `json:"replyNum"`
}

type Creator struct {
	StudentID string `json:"studentId"`
	Username  string `json:"username"`
	Avatar    string `json:"avatar"`
}

type ReplyCreator struct {
	StudentID string `json:"studentId"`
	Username  string `json:"username"`
	Avatar    string `json:"avatar"`
}
