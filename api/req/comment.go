package req

type DeleteCommentReq struct {
	TargetID string `json:"targetId" validate:"required"`
}

type CreateCommentReq struct {
	Content  string `json:"content" validate:"required"`
	ParentID string `json:"parentId" validate:"required"`
	Subject  string `json:"subject" validate:"required"`
}

type LoadCommentsReq struct {
	Id string `json:"id" validate:"required" form:"id" uri:"id"`
}
