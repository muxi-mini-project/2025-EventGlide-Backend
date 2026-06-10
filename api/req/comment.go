package req

import "github.com/raiki02/EG/pkg/utils"

type DeleteCommentReq struct {
	TargetID utils.SnowflakeID `json:"targetId" validate:"required"`
}

type CreateCommentReq struct {
	Content  string            `json:"content" validate:"required"`
	ParentID utils.SnowflakeID `json:"parentId" validate:"required"`
	Subject  string            `json:"subject" validate:"required"`
}

type LoadCommentsReq struct {
	Id utils.SnowflakeID `json:"id" validate:"required" form:"id" uri:"id"`
}