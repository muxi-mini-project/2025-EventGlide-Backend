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

type Image struct {
	Url    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
