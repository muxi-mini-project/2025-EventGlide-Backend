package resp

type Resp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type LoginResp struct {
	Id     int    `json:"Id"`
	Sid    string `json:"sid"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	School string `json:"school"`
	Likes  string `json:"likes"`
	Token  string `json:"token"`
}
