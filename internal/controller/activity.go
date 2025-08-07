package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type ActControllerHdl interface {
	NewAct() gin.HandlerFunc
	NewDraft() gin.HandlerFunc
	LoadDraft() gin.HandlerFunc
	FindActBySearches() gin.HandlerFunc
	FindActByName() gin.HandlerFunc
	FindActByDate() gin.HandlerFunc
	FindActByOwnerID() gin.HandlerFunc
	ListAllActs() gin.HandlerFunc
}

type ActController struct {
	as *service.ActivityService
	iu *service.ImgUploader
	l  *zap.Logger
}

func NewActController(as *service.ActivityService, iu *service.ImgUploader, l *zap.Logger) *ActController {
	return &ActController{
		as: as,
		iu: iu,
		l:  l.Named("activity/controller"),
	}
}

// @Tags Activity
// @Summary 创建活动
// @Produce json
// @Accept json
// @Param activity body req.CreateActReq true "活动"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.CreateActivityResp}
// @Router /act/create [post]
func (ac *ActController) NewAct() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			ac.l.Warn("request id is empty when create activity")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		var act req.CreateActReq
		//获取用户填写信息
		err := c.ShouldBindJSON(&act)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if act.Title == "" || act.Introduce == "" {
			c.JSON(200, tools.ReturnMSG(c, "你的参数有误，请重新输入！", nil))
			return
		}
		if len(act.LabelForm.Signer) <= 4 {
			c.JSON(200, tools.ReturnMSG(c, "请至少填写五个人的信息！", nil))
			return
		}

		if act.LabelForm.StartTime == "" || act.LabelForm.EndTime == "" {
			c.JSON(200, tools.ReturnMSG(c, "您填写的时间有误，请重新输入！", nil))
			return
		}

		if act.LabelForm.EndTime < act.LabelForm.StartTime {
			c.JSON(200, tools.ReturnMSG(c, "活动起始时间不能大于结束时间！", nil))
			return
		}
		act.StudentID = sid
		a, err := ac.as.NewAct(c, &act)
		if err != nil {
			ac.l.Error("create activity failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦！请稍后尝试！", nil))
			return
		}

		c.JSON(200, tools.ReturnMSG(c, "success", a))
	}
}

// @Tags Activity
// @Summary 创建活动草稿
// @Description not finished
// @Produce json
// @Accept json
// @Param draft body req.CreateActReq true "活动草稿"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=req.CreateActReq}
// @Router /act/draft [post]
func (ac *ActController) NewDraft() gin.HandlerFunc {
	return func(c *gin.Context) {
		var d req.CreateActReq
		//获取用户填写信息
		err := c.ShouldBindJSON(&d)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}

		_, err = ac.as.NewDraft(c, &d)
		if err != nil {
			ac.l.Error("create draft failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", d))
	}
}

// @Tags Activity
// @Summary 加载活动草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=model.ActivityDraft}
// @Router /act/load [get]
func (ac *ActController) LoadDraft() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := tools.GetSid(c)
		if sid == "" {
			ac.l.Warn("request id is empty when load draft")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		d, err := ac.as.LoadDraft(c, sid)
		if err != nil {
			ac.l.Error("load draft failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", d))
	}
}

// @Tags Activity
// @Summary 通过名称查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param name body req.FindActByNameReq true "活动名称"
// @Success 200 {object} resp.Resp{data=[]resp.ListActivitiesResp}
// @Router /act/name [post]
func (ac *ActController) FindActByName() gin.HandlerFunc {
	return func(c *gin.Context) {
		var r req.FindActByNameReq
		err := c.ShouldBindJSON(&r)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		if r.Name == "" {
			ac.l.Warn("request name is empty when find act by name")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		as, err := ac.as.FindActByName(c, r.Name)
		if err != nil {
			ac.l.Error("find act by name failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", as))
	}
}

// @Tags Activity
// @Summary 通过搜索条件查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param actSearchReq body req.ActSearchReq true "搜索条件"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/search [post]
func (ac *ActController) FindActBySearches() gin.HandlerFunc {
	return func(c *gin.Context) {
		var actReq req.ActSearchReq
		err := c.ShouldBindJSON(&actReq)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}
		as, err := ac.as.FindActBySearches(c, &actReq)
		if err != nil {
			ac.l.Error("find act by searches failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", as))
	}
}

// @Tags Activity
// @Summary 通过日期查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param date body  req.FindActByDateReq true "日期查找"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/date [post]
func (ac *ActController) FindActByDate() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 02-01
		var r req.FindActByDateReq

		err := c.ShouldBindJSON(&r)
		if err != nil {
			c.JSON(200, tools.ReturnMSG(c, err.Error(), nil))
			return
		}

		if r.Date == "" {
			ac.l.Warn("request date is empty when find act by date")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		as, err := ac.as.FindActByDate(c, r.Date)
		if err != nil {
			ac.l.Error("find act by date failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", as))
	}
}

// @Tags Activity
// @Summary 通过创建者id查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param id path string true "创建者id"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/own/{id} [get]
func (ac *ActController) FindActByOwnerID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			ac.l.Warn("request id is empty when find act by ownerid")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		as, err := ac.as.FindActByOwnerID(c, id)
		if err != nil {
			ac.l.Error("find act by ownerid failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", as))
	}
}

// @Tags Activity
// @Summary 列出所有活动
// @Produce json
// @Param Authorization header string true "token"
// @Param id path string true "用户id"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/all/{id} [get]
func (ac *ActController) ListAllActs() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			ac.l.Warn("request id is empty when list all acts")
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		as, err := ac.as.ListAllActs(c, id)
		if err != nil {
			ac.l.Error("list all acts failed", zap.Error(err))
			c.JSON(200, tools.ReturnMSG(c, "服务器出错啦, 请稍后尝试!", nil))
			return
		}
		c.JSON(200, tools.ReturnMSG(c, "success", as))
	}
}
