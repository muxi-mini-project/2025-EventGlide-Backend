package req

import "github.com/spf13/viper"

// todo 写在api的res/req中

type NumberReq struct {
	Topic string
	Sid   string `json:"sid"`
	Bid   string `json:"bid"`
}

type ActSearchReq struct {
	Type       []string `json:"type"`
	Host       []string `json:"host"`
	Location   []string `json:"location"`
	IfRegister string   `json:"if_register"`
	StartTime  string   `json:"start_time"`
	EndTime    string   `json:"end_time"`
}

func (a *ActSearchReq) GetTypes() []string {
	if len(a.Type) == 0 || len(a.Type) == viper.GetInt("actselect.max_type_num") {
		return nil
	}
	return a.Type
}

func (a *ActSearchReq) GetHosts() []string {
	if len(a.Host) == 0 || len(a.Host) == viper.GetInt("actselect.max_host_num") {
		return nil
	}
	return a.Host
}

func (a *ActSearchReq) GetLocations() []string {
	if len(a.Location) == 0 || len(a.Location) == viper.GetInt("actselect.max_location_num") {
		return nil
	}
	return a.Location
}

type UserSearchReq struct {
	Sid     string `json:"sid"`
	Keyword string `json:"keyword"`
}

type DraftReq struct {
	Sid string `json:"sid"`
	Bid string `json:"bid"`
}

type CommentReq struct {
	CreatorID string `json:"creator_id"`
	TargetID  string `json:"target_id"`
	Content   string `json:"content"`
}

type UserAvatarReq struct {
	Sid       string `json:"sid"`
	AvatarUrl string `json:"avatar_url"`
}

type UpdateNameReq struct {
	Sid  string `json:"sid"`
	Name string `json:"new_name"`
}
type LoginReq struct {
	Studentid string `json:"studentid"`
	Password  string `json:"password"`
}

type DeleteCommentReq struct {
	Sid      string `json:"sid"`
	TargetID string `json:"target_id"`
}

type FindCommentReq struct {
	Name string `json:"name"`
}
