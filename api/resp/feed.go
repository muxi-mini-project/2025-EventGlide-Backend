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
	StudentID string `json:"StudentID"`
	Avatar    string `json:"Avatar"`
	Username  string `json:"Username"`
}

type FeedLikeResp struct {
	Userinfo FeedUserInfo `json:"Userinfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"Message"`
	PublishedAt string            `json:"PublishedAt"`
	TargetId    utils.SnowflakeID `json:"TargetId"`
	RootID      utils.SnowflakeID `json:"RootId,omitempty"`
	RootType    string            `json:"RootType,omitempty"`
	Subject     string            `json:"Subject"`
	FirstPic    string            `json:"FirstPic,omitempty"`
	Status      string            `json:"status"`
}

type FeedCommentResp struct {
	Userinfo FeedUserInfo `json:"Userinfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"Message"`
	PublishedAt string            `json:"PublishedAt"`
	TargetId    utils.SnowflakeID `json:"TargetId"`
	RootID      utils.SnowflakeID `json:"RootId,omitempty"`
	RootType    string            `json:"RootType,omitempty"`
	Subject     string            `json:"Subject"`
	FirstPic    string            `json:"FirstPic,omitempty"`
	Status      string            `json:"status"`
}

type FeedAtResp struct {
	Userinfo FeedUserInfo `json:"Userinfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"Message"`
	PublishedAt string            `json:"PublishedAt"`
	TargetId    utils.SnowflakeID `json:"TargetId"`
	RootID      utils.SnowflakeID `json:"RootId,omitempty"`
	RootType    string            `json:"RootType,omitempty"`
	Subject     string            `json:"Subject"`
	FirstPic    string            `json:"FirstPic,omitempty"`
	Status      string            `json:"status"`
}

type FeedCollectResp struct {
	Userinfo FeedUserInfo `json:"Userinfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"Message"`
	PublishedAt string            `json:"PublishedAt"`
	FirstPic    string            `json:"FirstPic,omitempty"`
	TargetId    utils.SnowflakeID `json:"TargetId"`
	RootID      utils.SnowflakeID `json:"RootId,omitempty"`
	RootType    string            `json:"RootType,omitempty"`
	Subject     string            `json:"Subject"`
	Status      string            `json:"status"`
}

type FeedInvitationResp struct {
	Userinfo FeedUserInfo `json:"Userinfo"`

	Id          utils.SnowflakeID `json:"id"`
	Message     string            `json:"Message"`
	PublishedAt string            `json:"PublishedAt"`
	TargetId    utils.SnowflakeID `json:"TargetId"`
	RootID      utils.SnowflakeID `json:"RootId,omitempty"`
	RootType    string            `json:"RootType,omitempty"`
	Subject     string            `json:"Subject"`
	FirstPic    string            `json:"FirstPic,omitempty"`
	Status      string            `json:"status"`
}
