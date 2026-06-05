package req

type InteractionReq struct {
	TargetID int64 `json:"targetId" validate:"required"`
	Subject  string `json:"subject" validate:"required"`
}
