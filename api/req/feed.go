package req

import "github.com/raiki02/EG/pkg/utils"

type NumReq struct {
	TargetId utils.SnowflakeID `json:"targetId"`
	Object   string            `json:"object"`
	Action   string            `json:"action"`
}

type ReadFeedDetailReq struct {
	Id utils.SnowflakeID `json:"id" validate:"required" form:"id" uri:"id"`
}