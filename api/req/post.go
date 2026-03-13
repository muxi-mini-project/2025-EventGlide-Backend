package req

type CreatePostReq struct {
	Title     string   `json:"title" validate:"required"`
	Introduce string   `json:"introduce" validate:"required"`
	ShowImg   []string `json:"showImg" validate:"required,min=1"`
}

type CreatePostDraftReq struct {
	Title     string   `json:"title"`
	Introduce string   `json:"introduce"`
	ShowImg   []string `json:"showImg"`
}

type FindPostReq struct {
	Name string `json:"name" validate:"required"`
}

type DeletePostReq struct {
	TargetID string `json:"targetId" validate:"required"`
}
