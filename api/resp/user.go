package resp

type LoginResp struct {
	Id       int    `json:"Id"`
	Sid      string `json:"studentId"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	School   string `json:"school"`
	Token    string `json:"token"`
}

type UserInfoResp struct {
	Id       int    `json:"Id"`
	Sid      string `json:"studentId"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	School   string `json:"school"`
}
