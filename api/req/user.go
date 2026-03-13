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
}
