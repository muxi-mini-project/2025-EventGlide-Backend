package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/ginx"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

type FeedHandler struct {
	fs *service.FeedService
	l  *zap.Logger
}

func NewFeedHandler(e *gin.Engine, fs *service.FeedService, j *middleware.Jwt, l *logger.LoggerSet) *FeedHandler {
	f := &FeedHandler{
		fs: fs,
		l:  l.Feed.Named("handler"),
	}
	f.RegisterFeedHandlers(e, j.WrapCheckToken())

	return f
}

func (fh *FeedHandler) RegisterFeedHandlers(e *gin.Engine, handlerFunc gin.HandlerFunc) {
	feed := e.Group("/feed")
	feed.Use(handlerFunc)
	{
		feed.GET("/total", ginx.WrapWithClaims(fh.GetTotalCnt))
		feed.GET("/list", ginx.WrapWithClaims(fh.GetFeedList))
		feed.GET("/read/detail/:id", ginx.WrapRequestWithClaims(fh.ReadFeedDetail))
		feed.GET("/read/all", ginx.WrapWithClaims(fh.ReadAllFeed))
		feed.GET("/auditor", ginx.WrapWithClaims(fh.GetAuditorFeedList))
	}
}

// GetTotalCnt
// @Summary 获取用户的消息总数
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.BriefFeedResp}
// @Router /feed/total [get]
func (fh *FeedHandler) GetTotalCnt(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := fh.fs.GetTotalCnt(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// GetFeedList
// @Summary 获取feed列表
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.FeedResp}
// @Router /feed/list [get]
func (fh *FeedHandler) GetFeedList(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := fh.fs.GetFeedList(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// GetAuditorFeedList
// @Summary 获取审核员feed列表
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.FeedResp}
// @Router /feed/auditor [get]
func (fh *FeedHandler) GetAuditorFeedList(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := fh.fs.GetAuditorFeedList(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// ReadFeedDetail
// @Summary 读取feed详情, 标记已读
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Param id path string true "业务ID"
// @Success 200 {object} resp.Resp
// @Router /feed/read/detail/{id} [get]
func (fh *FeedHandler) ReadFeedDetail(ctx *gin.Context, req_ req.ReadFeedDetailReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := fh.fs.ReadFeedDetail(ctx, claims.Subject, int64(req_.Id)); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// ReadAllFeed
// @Summary 读取全部feed, 标记已读
// @Tags feed
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp
// @Router /feed/read/all [get]
func (fh *FeedHandler) ReadAllFeed(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := fh.fs.ReadAllFeed(ctx, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}
