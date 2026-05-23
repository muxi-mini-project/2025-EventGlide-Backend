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

type ActHandler struct {
	e  *gin.Engine
	as *service.ActivityService
	iu *service.ImgUploader
	j  *middleware.Jwt
	l  *zap.Logger
}

func NewActHandler(e *gin.Engine, as *service.ActivityService, iu *service.ImgUploader, l *zap.Logger, j *middleware.Jwt) *ActHandler {
	return &ActHandler{
		e:  e,
		as: as,
		iu: iu,
		j:  j,
		l:  l.Named("activity/handler"),
	}
}

func (ah *ActHandler) RegisterActHandlers() {
	act := ah.e.Group("act")
	act.Use(ah.j.WrapCheckToken())
	{
		act.POST("/create", ginx.WrapRequestWithClaims(ah.NewAct))
		act.POST("/draft", ginx.WrapRequestWithClaims(ah.NewDraft))
		act.GET("/load", ginx.WrapWithClaims(ah.LoadDraft))
		act.POST("/name", ginx.WrapRequest(ah.FindActByName))
		act.POST("/date", ginx.WrapRequest(ah.FindActByDate))
		act.POST("/search", ginx.WrapRequest(ah.FindActBySearches))
		act.GET("/own", ginx.WrapWithClaims(ah.FindActByOwnerID))
		act.GET("/all", ginx.WrapWithClaims(ah.ListAllActs))
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
func (ah *ActHandler) NewAct(ctx *gin.Context, req_ req.CreateActReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ah.as.NewAct(ctx, &req_, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Activity
// @Summary 创建活动草稿
// @Description not finished
// @Produce json
// @Accept json
// @Param draft body req.CreateActDraftReq true "活动草稿"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=req.CreateActDraftReq}
// @Router /act/draft [post]
func (ah *ActHandler) NewDraft(ctx *gin.Context, req_ req.CreateActDraftReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ah.as.NewDraft(ctx, &req_, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Activity
// @Summary 加载活动草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.LoadActivitiesDraftResp}
// @Router /act/load [get]
func (ah *ActHandler) LoadDraft(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	draft, err := ah.as.LoadDraft(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(ah.as.ToLoadDraftResp(draft))
}

// @Tags Activity
// @Summary 通过名称查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param name body req.FindActByNameReq true "活动名称"
// @Success 200 {object} resp.Resp{data=[]resp.ListActivitiesResp}
// @Router /act/name [post]
func (ah *ActHandler) FindActByName(ctx *gin.Context, req_ req.FindActByNameReq) (resp.Resp, error) {
	res, err := ah.as.FindActByName(ctx, req_.Name)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Activity
// @Summary 通过搜索条件查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param actSearchReq body req.ActSearchReq true "搜索条件"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/search [post]
func (ah *ActHandler) FindActBySearches(ctx *gin.Context, req_ req.ActSearchReq) (resp.Resp, error) {
	res, err := ah.as.FindActBySearches(ctx, &req_)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Activity
// @Summary 通过日期查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Param date body  req.FindActByDateReq true "日期查找"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/date [post]
func (ah *ActHandler) FindActByDate(ctx *gin.Context, req_ req.FindActByDateReq) (resp.Resp, error) {
	res, err := ah.as.FindActByDate(ctx, req_.Date)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Activity
// @Summary 通过创建者id查找活动
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/own [get]
func (ah *ActHandler) FindActByOwnerID(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ah.as.FindActByOwnerID(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Activity
// @Summary 列出所有活动
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.ListActivitiesResp}
// @Router /act/all [get]
func (ah *ActHandler) ListAllActs(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ah.as.ListAllActs(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}
