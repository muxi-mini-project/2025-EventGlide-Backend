package req

type NumReq struct {
	TargetId int64 `json:"targetId"`
	Object   string `json:"object"`
	Action   string `json:"action"`
}

type ReadFeedDetailReq struct {
	Id int64 `json:"id" validate:"required" form:"id" uri:"id"`
}