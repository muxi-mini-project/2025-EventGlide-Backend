package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type FeedControllerHdl interface {
	GetTotalCnt() func(c *gin.Context)
	GetFeedList() func(c *gin.Context)
}

type FeedController struct {
	fs *service.FeedService
	l  *zap.Logger
}

func NewFeedController(fs *service.FeedService, l *zap.Logger) *FeedController {
	return &FeedController{
		fs: fs,
		l:  l.Named("feed/controller"),
	}
}

// @Summary 获取用户的消息总数
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.BriefFeedResp}
// @Router /feed/total [get]
func (fc *FeedController) GetTotalCnt() func(c *gin.Context) {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			fc.l.Warn("request studentid is empty when get total comment")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		res, err := fc.fs.GetTotalCnt(c, sid)
		if err != nil {
			fc.l.Error("get total cnt failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}

// @Summary 获取feed列表
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.FeedResp}
// @Router /feed/list [get]
func (fc *FeedController) GetFeedList() func(c *gin.Context) {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			fc.l.Warn("request studentid is empty when get feed list")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		res, err := fc.fs.GetFeedList(c, sid)
		if err != nil {
			fc.l.Error("get feed list failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦,请稍后再尝试! ", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", res))
	}
}
