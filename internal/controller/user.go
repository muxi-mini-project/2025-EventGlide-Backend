package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type UserControllerHdl interface {
	Login() gin.HandlerFunc
	Logout() gin.HandlerFunc
	GetUserInfo() gin.HandlerFunc
	UpdateAvatar() gin.HandlerFunc
	UpdateUsername() gin.HandlerFunc
	SearchUserAct() gin.HandlerFunc
	SearchUserPost() gin.HandlerFunc
	GenQINIUToken() gin.HandlerFunc
	Like() gin.HandlerFunc
	Comment() gin.HandlerFunc
	Collect() gin.HandlerFunc
	LoadCollect() gin.HandlerFunc
}

type UserController struct {
	e   *gin.Engine
	ush *service.UserService
	l   *zap.Logger
}

func NewUserController(e *gin.Engine, ush *service.UserService, l *zap.Logger) *UserController {
	return &UserController{
		e:   e,
		ush: ush,
		l:   l.Named("user/controller"),
	}
}

// @Tags User
// @Summary 登录
// @Produce json
// @Param user body req.LoginReq true "登录请求"
// @Success 200 {object} resp.Resp{data=resp.LoginResp}
// @Router /user/login [post]
func (uc *UserController) Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		var lr req.LoginReq
		err := c.ShouldBindJSON(&lr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}

		if lr.StudentID == "" || lr.Password == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("账号或者密码为空", zap.String("studentID", lr.StudentID))
			return
		}
		user, token, err := uc.ush.Login(c, lr.StudentID, lr.Password)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("ccnu登录失败", zap.Error(err))
			return
		}
		res := resp.LoginResp{
			Id:     user.Id,
			Sid:    user.StudentID,
			Name:   user.Name,
			Avatar: user.Avatar,
			School: user.School,
			Token:  token,
		}

		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}

// @Tags User
// @Summary 登出
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp
// @Router /user/logout [post]
func (uc *UserController) Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		err := uc.ush.Logout(c, token)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("ccnu登出失败", zap.Error(err))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags User
// @Summary 获取用户信息
// @Produce json
// @Param Authorization header string true "token"
// @Param id path string true "用户id"
// @Success 200 {object} resp.Resp{data=model.User}
// @Router /user/info/{id} [get]
func (uc *UserController) GetUserInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := c.Param("id")
		if sid == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("sid", sid))
			return
		}
		user, err := uc.ush.GetUserInfo(c, sid)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("获取学生信息失败", zap.Error(err))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", user))
	}
}

// @Tags User
// @Summary 更新头像
// @Description not finished
// @Produce json
// @Param Authorization header string true "token"
// @Param userAvatarReq body req.UserAvatarReq true "用户头像更改"
// @Success 200 {object} resp.Resp
// @Router /user/avatar [post]
func (uc *UserController) UpdateAvatar() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			uc.l.Warn("request studentid is empty when update avatar")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var userAvatarReq req.UserAvatarReq
		err := c.ShouldBind(&userAvatarReq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if  userAvatarReq.AvatarUrl == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("头像url为空", zap.String("avatarUrl", userAvatarReq.AvatarUrl))
			return
		}
		userAvatarReq.StudentID=sid
		err = uc.ush.UpdateAvatar(c, userAvatarReq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("更新头像失败", zap.Error(err))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags User
// @Summary 更新用户名
// @Produce json
// @Param Authorization header string true "token"
// @Param unr body req.UpdateNameReq true "更新用户名"
// @Success 200 {object} resp.Resp
// @Router /user/username [post]
func (uc *UserController) UpdateUsername() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			uc.l.Warn("request studentid is empty when update username")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var unr req.UpdateNameReq
		err := c.ShouldBind(&unr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			uc.l.Error("绑定请求失败", zap.Error(err))
			return
		}
		if unr.Name == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("用户名为空", zap.String("studentID", sid))
			return
		}
		err = uc.ush.UpdateUsername(c, sid, unr.Name)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("更新用户名失败", zap.Error(err))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}

}

// @Tags User
// @Summary 搜索用户活动
// @Produce json
// @Param Authorization header string true "token"
// @Param ureq body req.UserSearchReq true "搜索请求"
// @Success 200 {object} resp.Resp{data=[]model.Activity}
// @Router /user/search/act [post]
func (uc *UserController) SearchUserAct() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ureq req.UserSearchReq
		err := c.ShouldBindJSON(&ureq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			uc.l.Error("绑定请求失败", zap.Error(err))
			return
		}
		if ureq.StudentID == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("studentID", ureq.StudentID))
			return
		}
		acts, err := uc.ush.SearchUserAct(c, ureq.StudentID, ureq.Keyword)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("搜索用户活动失败", zap.Error(err))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", acts))
	}
}

// @Tags User
// @Summary 搜索用户帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param ureq body req.UserSearchReq true "搜索请求"
// @Success 200 {object} resp.Resp{data=[]model.Post}
// @Router /user/search/post [post]
func (uc *UserController) SearchUserPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		var ureq req.UserSearchReq
		err := c.ShouldBindJSON(&ureq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			uc.l.Error("绑定请求失败", zap.Error(err))
			return
		}
		if ureq.StudentID == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("studentID", ureq.StudentID))
			return
		}
		posts, err := uc.ush.SearchUserPost(c, ureq.StudentID, ureq.Keyword)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("搜索用户帖子失败", zap.Error(err))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", posts))
	}
}

// @Tags User
// @Summary 获取七牛云token
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.ImgBedResp}
// @Router /user/token/qiniu [get]
func (uc *UserController) GenQiniuToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		res := uc.ush.GenQINIUToken(c)
		uc.l.Info("生成七牛云token", zap.String("token", res.AccessToken), zap.String("dn", res.DomainName))
		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}

// @Tags User
// @Summary 加载活动收藏
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "加载收藏请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListActivitiesResp}
// @Router /user/collect/act [post]
func (uc *UserController) LoadCollectAct() gin.HandlerFunc {
	return func(context *gin.Context) {
		var cr req.NumReq
		err := context.ShouldBindJSON(&cr)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, err.Error(), nil))
			uc.l.Error("绑定请求失败", zap.Error(err))
			return
		}
		if cr.StudentID == "" {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("studentID", cr.StudentID))
			return
		}
		res, err := uc.ush.LoadCollectAct(context, cr.StudentID)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("加载活动收藏失败", zap.Error(err))
			return
		}
		context.JSON(200, tools.ReturnMSG(context, "success", res))
	}
}

// @Tags User
// @Summary 加载帖子收藏
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "加载收藏请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /user/collect/post [post]
func (uc *UserController) LoadCollectPost() gin.HandlerFunc {
	return func(context *gin.Context) {
		var cr req.NumReq
		err := context.ShouldBindJSON(&cr)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, err.Error(), nil))
			uc.l.Error("绑定请求失败", zap.Error(err))
			return
		}
		if cr.StudentID == "" {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("studentID", cr.StudentID))
			return
		}
		res, err := uc.ush.LoadCollectPost(context, cr.StudentID)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("加载帖子收藏失败", zap.Error(err))
			return
		}
		context.JSON(200, tools.ReturnMSG(context, "success", res))
	}
}

// @Tags User
// @Summary 加载点赞过的帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "点赞请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /user/like/post [post]
func (uc *UserController) LoadLikePost() gin.HandlerFunc {
	return func(context *gin.Context) {
		var cr req.NumReq
		err := context.ShouldBindJSON(&cr)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, err.Error(), nil))
			uc.l.Error("绑定请求失败", zap.Error(err))
			return
		}
		if cr.StudentID == "" {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("studentID", cr.StudentID))
			return
		}
		res, err := uc.ush.LoadLikePost(context, cr.StudentID)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("加载帖子点赞失败", zap.Error(err))
			return
		}
		context.JSON(200, tools.ReturnMSG(context, "success", res))
	}
}

// @Tags User
// @Summary 加载点赞过的活动
// @Produce json
// @Param Authorization header string true "token"
// @Param cr body req.NumReq true "点赞请求"
// @Success 200 {object} resp.Resp{data=[]resp.ListActivitiesResp}
// @Router /user/like/act [post]
func (uc *UserController) LoadLikeAct() gin.HandlerFunc {
	return func(context *gin.Context) {
		var cr req.NumReq
		err := context.ShouldBindJSON(&cr)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, err.Error(), nil))
			return
		}
		if cr.StudentID == "" {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Warn("学生id为空", zap.String("studentID", cr.StudentID))
			return
		}
		res, err := uc.ush.LoadLikeAct(context, cr.StudentID)
		if err != nil {
			context.JSON(200, tools.ReturnMSG(context, "服务器出错啦, 请稍后尝试!", nil))
			uc.l.Error("加载活动点赞失败", zap.Error(err))
			return
		}
		context.JSON(200, tools.ReturnMSG(context, "success", res))
	}
}
