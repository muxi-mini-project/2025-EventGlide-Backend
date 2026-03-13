package req

type DraftReq struct {
	Bid string `json:"bid"`
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
