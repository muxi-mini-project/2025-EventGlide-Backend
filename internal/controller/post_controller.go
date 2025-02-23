package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
)

type PostControllerHdl interface {
	GetAllPost() gin.HandlerFunc
	CreatePost() gin.HandlerFunc
	FindPostByName() gin.HandlerFunc
	DeletePost() gin.HandlerFunc
	CreateDraft() gin.HandlerFunc
	LoadDraft() gin.HandlerFunc
}

type PostController struct {
	ps *service.PostService
}

func NewPostController(ps *service.PostService) *PostController {
	return &PostController{
		ps: ps,
	}
}

// @Tags Post
// @Summary 获取所有帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp
// @Router /post/all [get]
func (pc *PostController) GetAllPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		posts, err := pc.ps.GetAllPost(c)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
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
// @Param post body model.Post true "帖子"
// @Success 200 {object} resp.Resp
// @Router /post/create [post]
func (pc *PostController) CreatePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		var post model.Post
		err := c.ShouldBindJSON(&post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = pc.ps.CreatePost(c, &post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", post))
	}
}

// @Tags Post
// @Summary 通过帖子名查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param name body req.FindCommentReq true "帖子名"
// @Success 200 {object} resp.Resp
// @Router /post/find [post]
func (pc *PostController) FindPostByName() gin.HandlerFunc {
	return func(c *gin.Context) {

		var n req.FindCommentReq
		err := c.ShouldBindJSON(&n)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}

		posts, err := pc.ps.FindPostByName(c, n.Name)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
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
		var post model.Post
		err := c.ShouldBindJSON(&post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = pc.ps.DeletePost(c, &post)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
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
// @Param post body model.PostDraft true "草稿"
// @Success 200 {object} resp.Resp
// @Router /post/draft [post]
func (pr *PostController) CreateDraft() gin.HandlerFunc {
	return func(c *gin.Context) {
		var postDraft model.PostDraft
		err := c.ShouldBindJSON(&postDraft)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = pr.ps.CreateDraft(c, &postDraft)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", postDraft.Bid))
	}
}

// @Tags Post
// @Summary 加载草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param draft body req.DraftReq true "草稿请求"
// @Success 200 {object} resp.Resp
// @Router /post/load [post]
func (pr *PostController) LoadDraft() gin.HandlerFunc {
	return func(c *gin.Context) {
		var draftReq req.DraftReq
		err := c.ShouldBindJSON(&draftReq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		draft, err := pr.ps.LoadDraft(c, draftReq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", draft))
	}
}
