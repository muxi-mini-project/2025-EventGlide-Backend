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
	Name  string `json:"name" validate:"required"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

type DeletePostReq struct {
	TargetID string `json:"targetId" validate:"required"`
}

type FindPostByBidReq struct {
	Id string `json:"id" validate:"required" form:"id" uri:"id"`
}

type FindByUserIDReq struct {
	UserID string `json:"userId" validate:"required" form:"userId" uri:"userId"`
	Page   int    `json:"page,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type ListAllPostsReq struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type FindPostByOwnerIDReq struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type GetAllPostsReq struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}
