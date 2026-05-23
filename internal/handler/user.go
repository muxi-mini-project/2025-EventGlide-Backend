package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/ginx"
	"go.uber.org/zap"
)

type UserHandler struct {
	us *service.UserService
	l  *zap.Logger
}

func NewUserHandler(e *gin.Engine, us *service.UserService, j *middleware.Jwt, l *zap.Logger) *UserHandler {
	u := &UserHandler{
		us: us,
		l:  l.Named("user/handler"),
	}
	u.RegisterUserHandlers(e, j.WrapCheckToken())

	return u
}

func (uh *UserHandler) RegisterUserHandlers(e *gin.Engine, handlerFunc gin.HandlerFunc) {
	user := e.Group("/user")
	{
		user.POST("/login", ginx.WrapRequest(uh.Login))

		user.Use(handlerFunc)
		{
			user.POST("/logout", ginx.Wrap(uh.Logout))
			user.GET("/token/qiniu", ginx.Wrap(uh.GenQiniuToken))
			user.GET("/info/:id", ginx.WrapRequest(uh.GetUserInfo))
			user.POST("/avatar", ginx.WrapRequestWithClaims(uh.UpdateAvatar))
			user.POST("/username", ginx.WrapRequestWithClaims(uh.UpdateUsername))
			user.POST("/search/act", ginx.WrapRequestWithClaims(uh.SearchUserAct))
			user.POST("/search/post", ginx.WrapRequestWithClaims(uh.SearchUserPost))
			user.POST("/collect/act", ginx.WrapWithClaims(uh.LoadCollectAct))
			user.POST("/collect/post", ginx.WrapWithClaims(uh.LoadCollectPost))
			user.POST("/like/act", ginx.WrapWithClaims(uh.LoadLikeAct))
			user.POST("/like/post", ginx.WrapWithClaims(uh.LoadLikePost))
			user.GET("/checking", ginx.WrapWithClaims(uh.Checking))
		}
	}
}

// @Tags User
// @Summary 登录
// @Produce json
// @Param user body req.LoginReq true "登录请求"
// @Success 200 {object} resp.Resp{data=resp.LoginResp}
// @Router /user/login [post]
func (uh *UserHandler) Login(ctx *gin.Context, req_ req.LoginReq) (resp.Resp, error) {
	user, token, err := uh.us.Login(ctx, req_.StudentID, req_.Password)
	if err != nil {
		return ginx.ReturnError(err)
	}
	res := resp.LoginResp{
		Id:       user.Id,
		Sid:      user.StudentID,
		Username: user.Name,
		Avatar:   user.Avatar,
		College:  user.College,
		School:   user.School,
		Token:    token,
	}

	return ginx.ReturnSuccess(res)
}

// @Tags User
// @Summary 登出
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp
// @Router /user/logout [post]
func (uh *UserHandler) Logout(ctx *gin.Context) (resp.Resp, error) {
	token := ctx.GetHeader("Authorization")
	if err := uh.us.Logout(ctx, token); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags User
// @Summary 获取用户信息
// @Produce json
// @Param Authorization header string true "token"
// @Param id path string true "用户id"
// @Success 200 {object} resp.Resp{data=resp.UserInfoResp}
// @Router /user/info/{id} [get]
func (uh *UserHandler) GetUserInfo(ctx *gin.Context, req_ req.GetUserInfoReq) (resp.Resp, error) {
	res, err := uh.us.GetUserInfo(ctx, req_.Id)
	if err != nil {
		return ginx.ReturnError(err)
	}

	resp := resp.UserInfoResp{
		College:  res.College,
		Id:       res.Id,
		Sid:      res.StudentID,
		Username: res.Name,
		Avatar:   res.Avatar,
		School:   res.School,
	}

	return ginx.ReturnSuccess(resp)
}

// @Tags User
// @Summary 更新头像
// @Description not finished
// @Produce json
// @Param Authorization header string true "token"
// @Param userAvatarReq body req.UserAvatarReq true "用户头像更改"
// @Success 200 {object} resp.Resp
// @Router /user/avatar [post]
func (uh *UserHandler) UpdateAvatar(ctx *gin.Context, req_ req.UserAvatarReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := uh.us.UpdateAvatar(ctx, req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags User
// @Summary 更新用户名
// @Produce json
// @Param Authorization header string true "token"
// @Param unr body req.UpdateNameReq true "更新用户名"
// @Success 200 {object} resp.Resp
// @Router /user/username [post]
func (uh *UserHandler) UpdateUsername(ctx *gin.Context, req_ req.UpdateNameReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := uh.us.UpdateUsername(ctx, claims.Subject, req_.Name); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags User
// @Summary 搜索用户活动
// @Produce json
// @Param Authorization header string true "token"
// @Param ureq body req.UserSearchReq true "搜索请求"
// @Success 200 {object} resp.Resp{data=[]model.Activity}
// @Router /user/search/act [post]
func (uh *UserHandler) SearchUserAct(ctx *gin.Context, req_ req.UserSearchReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	acts, err := uh.us.SearchUserAct(ctx, claims.Subject, req_.Keyword)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(acts)
}

// @Tags User
// @Summary 搜索用户帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param ureq body req.UserSearchReq true "搜索请求"
// @Success 200 {object} resp.Resp{data=[]model.Post}
// @Router /user/search/post [post]
func (uh *UserHandler) SearchUserPost(ctx *gin.Context, req_ req.UserSearchReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := uh.us.SearchUserPost(ctx, claims.Subject, req_.Keyword)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(posts)
}

// @Tags User
// @Summary 获取七牛云token
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.ImgBedResp}
// @Router /user/token/qiniu [get]
func (uh *UserHandler) GenQiniuToken(ctx *gin.Context) (resp.Resp, error) {
	res := uh.us.GenQINIUToken(ctx)
	return ginx.ReturnSuccess(res)
}

// @Tags User
// @Summary 加载活动收藏
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "加载收藏请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListActivitiesResp}
// @Router /user/collect/act [post]
func (uh *UserHandler) LoadCollectAct(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := uh.us.LoadCollectAct(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags User
// @Summary 加载帖子收藏
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "加载收藏请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /user/collect/post [post]
func (uh *UserHandler) LoadCollectPost(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := uh.us.LoadCollectPost(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags User
// @Summary 加载点赞过的帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "点赞请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /user/like/post [post]
func (uh *UserHandler) LoadLikePost(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := uh.us.LoadLikePost(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags User
// @Summary 加载点赞过的活动
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "点赞请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListActivitiesResp}
// @Router /user/like/act [post]
func (uh *UserHandler) LoadLikeAct(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := uh.us.LoadLikeAct(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags User
// @Summary 获取用户处于审核状态中的活动和帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.CheckingResp}
// @Router /user/checking [get]
func (uh *UserHandler) Checking(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := uh.us.GetChecking(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}
