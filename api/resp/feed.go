package resp

import "github.com/raiki02/EG/pkg/utils"

type BriefFeedResp struct {
	LikeAndCollect int `json:"likeAndCollect"`
	CommentAndAt   int `json:"commentAndAt"`
	Total          int `json:"total"`
}

type FeedResp struct {
	Likes       []FeedLikeResp       `json:"Likes,omitempty"`
	Ats         []FeedAtResp         `json:"Ats,omitempty"`
	Comments    []FeedCommentResp    `json:"Comments,omitempty"`
	Collects    []FeedCollectResp    `json:"Collects,omitempty"`
	Invitations []FeedInvitationResp `json:"Invitations,omitempty"`
}

type FeedUserInfo struct {
	StudentID string `json:"studentId"`
	Avatar    string `json:"avatar"`
	Username  string `json:"username"`
}

type FeedLikeResp struct {
	Userinfo FeedUserInfo `json:"userInfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"message"`
	PublishedAt string            `json:"publishedAt"`
	TargetId    utils.SnowflakeID `json:"targetId"`
	RootID      utils.SnowflakeID `json:"rootId,omitempty"`
	RootType    string            `json:"rootType,omitempty"`
	Subject     string            `json:"subject"`
	FirstPic    string            `json:"firstPic,omitempty"`
	Status      string            `json:"status"`
}

type FeedCommentResp struct {
	Userinfo FeedUserInfo `json:"userInfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"message"`
	PublishedAt string            `json:"publishedAt"`
	TargetId    utils.SnowflakeID `json:"targetId"`
	RootID      utils.SnowflakeID `json:"rootId,omitempty"`
	RootType    string            `json:"rootType,omitempty"`
	Subject     string            `json:"subject"`
	FirstPic    string            `json:"firstPic,omitempty"`
	Status      string            `json:"status"`
}

type FeedAtResp struct {
	Userinfo FeedUserInfo `json:"userInfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"message"`
	PublishedAt string            `json:"publishedAt"`
	TargetId    utils.SnowflakeID `json:"targetId"`
	RootID      utils.SnowflakeID `json:"rootId,omitempty"`
	RootType    string            `json:"rootType,omitempty"`
	Subject     string            `json:"subject"`
	FirstPic    string            `json:"firstPic,omitempty"`
	Status      string            `json:"status"`
}

type FeedCollectResp struct {
	Userinfo FeedUserInfo `json:"userInfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"message"`
	PublishedAt string            `json:"publishedAt"`
	FirstPic    string            `json:"firstPic,omitempty"`
	TargetId    utils.SnowflakeID `json:"targetId"`
	RootID      utils.SnowflakeID `json:"rootId,omitempty"`
	RootType    string            `json:"rootType,omitempty"`
	Subject     string            `json:"subject"`
	Status      string            `json:"status"`
}

type FeedInvitationResp struct {
	Userinfo FeedUserInfo `json:"userInfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"message"`
	PublishedAt string            `json:"publishedAt"`
	TargetId    utils.SnowflakeID `json:"targetId"`
	RootID      utils.SnowflakeID `json:"rootId,omitempty"`
	RootType    string            `json:"rootType,omitempty"`
	Subject     string            `json:"subject"`
	FirstPic    string            `json:"firstPic,omitempty"`
	Status      string            `json:"status"`
}
