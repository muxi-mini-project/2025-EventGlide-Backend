package req

type InteractionReq struct {
	TargetID string `json:"targetId" validate:"required"`
	Subject  string `json:"subject" validate:"required"`
}
