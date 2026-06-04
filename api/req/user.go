package req

type UpdateNameReq struct {
	Name string `json:"newName" validate:"required"`
}
type LoginReq struct {
	StudentID string `json:"studentId" validate:"required,len=10"`
	Password  string `json:"password" validate:"required"`
}

type UserSearchReq struct {
	Keyword string `json:"keyword"`
	Page    int    `json:"page,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type VerifyUserReq struct {
	StudentID string `json:"studentId" validate:"required,len=10"`
	RealName  string `json:"realName" validate:"required"`
}
