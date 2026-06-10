package resp

import "github.com/raiki02/EG/pkg/utils"

type CommentResp struct {
	Id utils.SnowflakeID `json:"id"`

	Creator Creator `json:"creator"`

	CommentedTime string `json:"commentedTime"`
	CommentedPos  string `json:"commentedPos"`
	Content       string `json:"content"`
	LikeNum       int    `json:"likeNum"`
	ReplyNum      int    `json:"replyNum"`
	IsLike        string `json:"isLike"`
	ParentID      utils.SnowflakeID `json:"parentId"`
	RootID        utils.SnowflakeID `json:"rootId"`

	Reply []ReplyResp `json:"reply"`
}

type ReplyResp struct {
	Id utils.SnowflakeID `json:"id"`

	ReplyCreator ReplyCreator `json:"replyCreator"`

	ReplyContent string `json:"replyContent"`
	ReplyTime    string `json:"replyTime"`
	ReplyPos     string `json:"replyPos"`

	ParentID       utils.SnowflakeID `json:"parentId"`
	RootID         utils.SnowflakeID `json:"rootId"`
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