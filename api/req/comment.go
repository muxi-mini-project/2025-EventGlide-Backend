package req

type DeleteCommentReq struct {
	TargetID int64 `json:"targetId" validate:"required"`
}

type CreateCommentReq struct {
	Content  string `json:"content" validate:"required"`
	ParentID int64 `json:"parentId" validate:"required"`
	Subject  string `json:"subject" validate:"required"`
}

type LoadCommentsReq struct {
	Id int64 `json:"id" validate:"required" form:"id" uri:"id"`
}