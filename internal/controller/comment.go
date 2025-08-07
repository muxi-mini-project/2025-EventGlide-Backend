package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type CommentControllerHdl interface {
	CreateComment() gin.HandlerFunc
	DeleteComment() gin.HandlerFunc
	AnswerComment() gin.HandlerFunc
	LoadComments() gin.HandlerFunc
	LoadAnswers() gin.HandlerFunc
}

type CommentController struct {
	cs *service.CommentService
	l  *zap.Logger
}

func NewCommentController(cs *service.CommentService, l *zap.Logger) *CommentController {
	return &CommentController{
		cs: cs,
		l:  l.Named("comment/controller"),
	}
}

// @Tags Comment
// @Summary 创建评论
// @Produce json
// @Param Authorization header string true "token"
// @Param CommentReq body req.CreateCommentReq true "评论"
// @Success 200 {object} resp.Resp{data=resp.CommentResp}
// @Router /comment/create [post]
func (cc *CommentController) CreateComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			cc.l.Warn("request id is empty when create comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		var r req.CreateCommentReq
		err := c.ShouldBindJSON(&r)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if r.Content == "" || r.ParentID == "" {
			cc.l.Warn("request  content or parentid is empty when create comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		res, err := cc.cs.CreateComment(c, r)
		if err != nil {
			cc.l.Error("create comment failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}

// @Tags Comment
// @Summary 回复评论
// @Produce json
// @Param Authorization header string true "token"
// @Param CommentReq body req.CreateCommentReq true "回复"
// @Success 200 {object} resp.Resp{data=resp.ReplyResp}
// @Router /comment/answer [post]
func (cc *CommentController) AnswerComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			cc.l.Warn("request id is empty when answer comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		var r req.CreateCommentReq
		err := c.ShouldBindJSON(&r)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if r.Content == "" || r.ParentID == "" {
			cc.l.Warn("request  content or parentid is empty when answer comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		r.StudentID=sid
		res, err := cc.cs.AnswerComment(c, r)
		if err != nil {
			cc.l.Error("answer comment failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}

// @Tags Comment
// @Summary 删除评论
// @Produce json
// @Param sid formData string true "学号"
// @Param Authorization header string true "token"
// @Param DeleteCommentReq body req.DeleteCommentReq true "删除评论"
// @Success 200 {object} resp.Resp
// @Router /comment/delete [post]
func (cc *CommentController) DeleteComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			cc.l.Warn("request id is empty when delete comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		var r req.DeleteCommentReq
		err := c.ShouldBindJSON(&r)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if r.TargetID == "" {
			cc.l.Warn("request targetid is empty when delete comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		r.StudentID=sid
		err = cc.cs.DeleteComment(c, r)
		if err != nil {
			cc.l.Error("delete comment failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags Comment
// @Summary 加载评论
// @Produce json
// @Param id path string true "目标id"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.CommentResp}
// @Router /comment/load/{id} [get]
func (cc *CommentController) LoadComments() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			cc.l.Warn("request id is empty when load comments")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		res, err := cc.cs.LoadComments(c, id)
		if err != nil {
			cc.l.Error("load comments failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}
