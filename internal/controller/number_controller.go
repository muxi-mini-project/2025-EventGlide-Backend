package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
)

type NumberControllerHdl interface {
	AddLikesNum() gin.HandlerFunc
	CutLikesNum() gin.HandlerFunc
	AddCommentsNum() gin.HandlerFunc
}

type NumberController struct {
	ns *service.NumberService
}

func NewNumberController(ns *service.NumberService) *NumberController {
	return &NumberController{
		ns: ns,
	}
}

// @Tags number
// @Summary 增加点赞数
// @Accept json
// @Param Authorization header string true "token"
// @Param number body model.Number true "点赞入参"
// @Success 200 {object} resp.Resp
// @Router /number/like [post]
func (nc *NumberController) AddLikesNum() gin.HandlerFunc {
	return func(c *gin.Context) {
		var nr model.Number
		err := c.ShouldBindJSON(&nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = nc.ns.AddLikesNum(c, &nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags number
// @Summary 减少点赞数
// @Accept json
// @Param Authorization header string true "token"
// @Param number body model.Number true "点赞入参"
// @Success 200 {object} resp.Resp
// @Router /number/unlike [post]
func (nc *NumberController) CutLikesNum() gin.HandlerFunc {
	return func(c *gin.Context) {
		var nr model.Number
		err := c.ShouldBindJSON(&nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = nc.ns.CutLikesNum(c, &nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags number
// @Summary 增加评论数
// @Accept json
// @Param Authorization header string true "token"
// @Param number body model.Number true "评论入参"
// @Success 200 {object} resp.Resp
// @Router /number/comment [post]
func (nc *NumberController) AddCommentsNum() gin.HandlerFunc {
	return func(c *gin.Context) {
		var nr model.Number
		err := c.ShouldBindJSON(&nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = nc.ns.AddCommentsNum(c, &nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}

// @Tags number
// @Summary 更新点赞数和评论数
// @Accept json
// @Param Authorization header string true "token"
// @Param number body model.Number true "更新入参"
// @Success 200 {object} resp.Resp
// @Router /number/update [post]
func (nc *NumberController) UpdateNumbers() gin.HandlerFunc {
	return func(c *gin.Context) {
		var nr model.Number
		err := c.ShouldBindJSON(&nr)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		err = nc.ns.UpdateNumbers(c, nr.Sid, nr.Bid)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", nil))
	}
}
