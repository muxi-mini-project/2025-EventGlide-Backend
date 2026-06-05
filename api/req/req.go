package req

type DraftReq struct {
	Id int64 `json:"id"`
}

type UserAvatarReq struct {
	AvatarUrl string `json:"avatarUrl" validate:"required,url"`
}

type AuditWrapper struct {
	Subject   string
	StudentId string

	CactReq  *CreateActReq
	CpostReq *CreatePostReq
}

type GetUserInfoReq struct {
	Id string `json:"id" validate:"required" form:"id" uri:"id"`
}