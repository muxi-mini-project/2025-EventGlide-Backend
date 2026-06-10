package req

import "github.com/raiki02/EG/pkg/utils"

type InteractionReq struct {
	TargetID utils.SnowflakeID `json:"targetId" validate:"required"`
	Subject  string            `json:"subject" validate:"required"`
}
