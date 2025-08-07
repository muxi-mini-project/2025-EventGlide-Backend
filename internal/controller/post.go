package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type PostControllerHdl interface {
	GetAllPost() gin.HandlerFunc
	CreatePost() gin.HandlerFunc
	FindPostByName() gin.HandlerFunc
	DeletePost() gin.HandlerFunc
	CreateDraft() gin.HandlerFunc
	LoadDraft() gin.HandlerFunc
	FindPostByOwnerID() gin.HandlerFunc
}

type PostController struct {
	ps *service.PostService
	l  *zap.Logger
}

func NewPostController(ps *service.PostService, l *zap.Logger) *PostController {
	return &PostController{
		ps: ps,
		l:  l.Named("post/controller"),
	}
}

// @Tags Post
// @Summary 获取所有帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/all [get]
func (pc *PostController) GetAllPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		posts, err := pc.ps.GetAllPost(c)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", posts))
	}
}

// @Tags Post
// @Summary 创建帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body req.CreatePostReq true "帖子"
// @Success 200 {object} resp.Resp{}
// @Router /post/create [post]
func (pc *PostController) CreatePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			pc.l.Warn("request studentid is empty when create post")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var post req.CreatePostReq
		err := c.ShouldBindJSON(&post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		post.StudentID=sid
		_, err = pc.ps.CreatePost(c, &post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags Post
// @Summary 通过帖子名查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param name body req.FindPostReq true "帖子名"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/find [post]
func (pc *PostController) FindPostByName() gin.HandlerFunc {
	return func(c *gin.Context) {
		var n req.FindPostReq
		err := c.ShouldBindJSON(&n)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}

		posts, err := pc.ps.FindPostByName(c, n.Name)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", posts))
	}
}

// @Tags Post
// @Summary 删除帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body model.Post true "帖子"
// @Success 200 {object} resp.Resp
// @Router /post/delete [post]
func (pc *PostController) DeletePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			pc.l.Warn("request studentid is empty when delete post")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var post model.Post
		err := c.ShouldBindJSON(&post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		post.StudentID=sid
		err = pc.ps.DeletePost(c, &post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags Post
// @Summary 创建草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body req.CreatePostReq true "草稿"
// @Success 200 {object} resp.Resp{data=req.CreatePostReq}
// @Router /post/draft [post]
func (pr *PostController) CreateDraft() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			pr.l.Warn("request studentid is empty when create draft")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var postDraft req.CreatePostReq
		err := c.ShouldBindJSON(&postDraft)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		postDraft.StudentID=sid
		_, err = pr.ps.CreateDraft(c, &postDraft)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", postDraft))
	}
}

// @Tags Post
// @Summary 加载草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=model.PostDraft}
// @Router /post/load [get]
func (pr *PostController) LoadDraft() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		draft, err := pr.ps.LoadDraft(c, sid)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", draft))
	}
}

// @Tags Post
// @Summary 通过用户ID查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param id path string true "用户ID"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/own/{id} [get]
func (pr *PostController) FindPostByOwnerID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		posts, err := pr.ps.FindPostByOwnerID(c, id)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", posts))
	}
}
