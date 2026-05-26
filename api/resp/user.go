package resp

type LoginResp struct {
	Id       int    `json:"Id"`
	Sid      string `json:"studentId"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	School   string `json:"school"`
	College  string `json:"college"`
	Token    string `json:"token"`
}

type UserInfoResp struct {
	College  string `json:"college"`
	Id       int    `json:"Id"`
	Sid      string `json:"studentId"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	School   string `json:"school"`
}

type UserInfo struct {
	StudentID string `json:"studentId"`
	Avatar    string `json:"avatar"`
	Username  string `json:"username"`
	School    string `json:"school"`
}
