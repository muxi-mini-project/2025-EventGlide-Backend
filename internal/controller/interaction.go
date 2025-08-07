package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type InteractionController struct {
	is *service.InteractionService
	l  *zap.Logger
}

func NewInteractionController(is *service.InteractionService, l *zap.Logger) *InteractionController {
	return &InteractionController{
		is: is,
		l:  l.Named("interaction/controller"),
	}
}

// @Tags Interaction
// @Summary 点赞
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/like [post]
func (ic *InteractionController) Like() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			ic.l.Warn("request studentid is empty when like action")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var ireq req.InteractionReq
		err := c.ShouldBindJSON(&ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if ireq.TargetID == "" || ireq.Subject == "" {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		ireq.StudentID=sid
		err = ic.is.Like(c, &ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags Interaction
// @Summary 取消点赞
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/dislike [post]
func (ic *InteractionController) Dislike() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			ic.l.Warn("request studentid is empty when dislike action")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var ireq req.InteractionReq
		err := c.ShouldBindJSON(&ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if ireq.TargetID == "" || ireq.Subject == "" {
			c.JSON(200, tools.ReturnMSG(c, "你的参数有误, 请重新输入!", nil))
			return
		}
		ireq.StudentID=sid
		err = ic.is.Dislike(c, &ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags Interaction
// @Summary 收藏
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/collect [post]
func (ic *InteractionController) Collect() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			ic.l.Warn("request studentid is empty when save action")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var ireq req.InteractionReq
		err := c.ShouldBindJSON(&ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if ireq.TargetID == "" || ireq.Subject == "" {
			c.JSON(200, tools.ReturnMSG(c, "你的参数有误, 请重新输入!", nil))
			return
		}
		ireq.StudentID=sid
		err = ic.is.Collect(c, &ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags Interaction
// @Summary 取消收藏
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/discollect [post]
func (ic *InteractionController) Discollect() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			ic.l.Warn("request studentid is empty when discollect action")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		var ireq req.InteractionReq
		err := c.ShouldBindJSON(&ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if ireq.TargetID == "" || ireq.Subject == "" {
			c.JSON(200, tools.ReturnMSG(c, "你的参数有误, 请重新输入!", nil))
			return
		}
		ireq.StudentID=sid
		err = ic.is.Discollect(c, &ireq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}
