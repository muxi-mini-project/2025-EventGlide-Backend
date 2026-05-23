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

type InteractionHandler struct {
	e  *gin.Engine
	is *service.InteractionService
	j  *middleware.Jwt
	l  *zap.Logger
}

func NewInteractionHandler(e *gin.Engine, is *service.InteractionService, j *middleware.Jwt, l *zap.Logger) *InteractionHandler {
	return &InteractionHandler{
		e:  e,
		is: is,
		j:  j,
		l:  l.Named("interaction/handler"),
	}
}

func (ih *InteractionHandler) RegisterInteractionHandlers() {
	i := ih.e.Group("interaction")
	i.Use(ih.j.WrapCheckToken())
	{
		i.POST("/like", ginx.WrapRequestWithClaims(ih.Like))
		i.POST("/dislike", ginx.WrapRequestWithClaims(ih.Dislike))

		i.POST("/collect", ginx.WrapRequestWithClaims(ih.Collect))
		i.POST("/discollect", ginx.WrapRequestWithClaims(ih.Discollect))

		i.POST("/approve", ginx.WrapRequestWithClaims(ih.Approve))
		i.POST("/reject", ginx.WrapRequestWithClaims(ih.Reject))
	}
}

// @Tags Interaction
// @Summary 点赞
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/like [post]
func (ih *InteractionHandler) Like(ctx *gin.Context, req_ req.InteractionReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ih.is.Like(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Interaction
// @Summary 取消点赞
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/dislike [post]
func (ih *InteractionHandler) Dislike(ctx *gin.Context, req_ req.InteractionReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ih.is.Dislike(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Interaction
// @Summary 收藏
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/collect [post]
func (ih *InteractionHandler) Collect(ctx *gin.Context, req_ req.InteractionReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ih.is.Collect(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Interaction
// @Summary 取消收藏
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/discollect [post]
func (ih *InteractionHandler) Discollect(ctx *gin.Context, req_ req.InteractionReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ih.is.Discollect(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Interaction
// @Summary 作为活动填表人批准发表此活动
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/approve [post]
func (ih *InteractionHandler) Approve(ctx *gin.Context, req_ req.InteractionReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ih.is.Approve(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Interaction
// @Summary 作为活动填表人拒绝发表此活动
// @Accept json
// @Param Authorization header string true "token"
// @Param interaction body req.InteractionReq true "互动"
// @Success 200 {object} resp.Resp
// @Router /interaction/reject [post]
func (ih *InteractionHandler) Reject(ctx *gin.Context, req_ req.InteractionReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ih.is.Reject(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}
