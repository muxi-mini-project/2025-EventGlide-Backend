package converter

import (
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
)

func ToLoginResp(user *model.User, token string) resp.LoginResp {
	return resp.LoginResp{
		Id:       user.Id,
		Sid:      user.StudentID,
		Username: user.Name,
		Avatar:   user.Avatar,
		College:  user.College,
		School:   user.School,
		Token:    token,
	}
}

func ToUserInfoResp(user *model.User) resp.UserInfoResp {
	return resp.UserInfoResp{
		College:  user.College,
		Id:       user.Id,
		Sid:      user.StudentID,
		Username: user.Name,
		Avatar:   user.Avatar,
		School:   user.School,
	}
}

func ToImgBedResp(token, domain string) resp.ImgBedResp {
	return resp.ImgBedResp{
		AccessToken: token,
		DomainName:  domain,
	}
}

func ToCheckingResp(acts []model.ActivityDetail, posts []model.PostDetail) resp.CheckingResp {
	return resp.CheckingResp{
		Acts:  ToListActivitiesResp(acts),
		Posts: ToListPostsResp(posts),
	}
}
