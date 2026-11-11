package req

import "encoding/json"

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
	Id string `json:"id" validate:"required" form:"-" uri:"id"`
}

type Image struct {
	Url    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// UnmarshalJSON 兼容旧版 showImg：数组元素可能仍是纯 URL 字符串。
// 字符串按「URL 已知、宽高未知」处理，避免旧客户端在创建/草稿接口直接 400；
// 对象格式与 null 的解析行为保持不变。
func (i *Image) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*i = Image{}
		return nil
	}
	var url string
	if err := json.Unmarshal(data, &url); err == nil {
		*i = Image{Url: url}
		return nil
	}
	type imageAlias Image
	var v imageAlias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*i = Image(v)
	return nil
}
