package resp

type Resp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type ImgBedResp struct {
	DomainName  string `json:"domainName"`
	AccessToken string `json:"accessToken"`
}

type CheckingResp struct {
	Acts  []ListActivitiesResp
	Posts []ListPostsResp
}
