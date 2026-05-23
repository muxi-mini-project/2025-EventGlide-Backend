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

type CommentHandler struct {
	e  *gin.Engine
	cs *service.CommentService
	j  *middleware.Jwt
	l  *zap.Logger
}

func NewCommentHandler(e *gin.Engine, cs *service.CommentService, j *middleware.Jwt, l *zap.Logger) *CommentHandler {
	return &CommentHandler{
		e:  e,
		cs: cs,
		j:  j,
		l:  l.Named("comment/handler"),
	}
}

func (ch *CommentHandler) RegisterCommentHandler() {
	cmt := ch.e.Group("/comment")
	cmt.Use(ch.j.WrapCheckToken())
	{
		cmt.POST("/create", ginx.WrapRequestWithClaims(ch.CreateComment))
		cmt.POST("/delete", ginx.WrapRequestWithClaims(ch.DeleteComment))
		cmt.POST("/answer", ginx.WrapRequestWithClaims(ch.AnswerComment))
		cmt.GET("/load/:id", ginx.WrapRequestWithClaims(ch.LoadComments))
	}
}

// @Tags Comment
// @Summary 创建评论
// @Produce json
// @Param Authorization header string true "token"
// @Param CommentReq body req.CreateCommentReq true "评论"
// @Success 200 {object} resp.Resp{data=resp.CommentResp}
// @Router /comment/create [post]
func (ch *CommentHandler) CreateComment(ctx *gin.Context, req_ req.CreateCommentReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ch.cs.CreateComment(ctx, req_, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Comment
// @Summary 回复评论
// @Produce json
// @Param Authorization header string true "token"
// @Param CommentReq body req.CreateCommentReq true "回复"
// @Success 200 {object} resp.Resp{data=resp.ReplyResp}
// @Router /comment/answer [post]
func (ch *CommentHandler) AnswerComment(ctx *gin.Context, req_ req.CreateCommentReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ch.cs.AnswerComment(ctx, req_, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Comment
// @Summary 删除评论
// @Produce json
// @Param Authorization header string true "token"
// @Param DeleteCommentReq body req.DeleteCommentReq true "删除评论"
// @Success 200 {object} resp.Resp
// @Router /comment/delete [post]
func (ch *CommentHandler) DeleteComment(ctx *gin.Context, req_ req.DeleteCommentReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	err := ch.cs.DeleteComment(ctx, req_.TargetID, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Comment
// @Summary 加载评论
// @Produce json
// @Param id path string true "目标id"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.CommentResp}
// @Router /comment/load/{id} [get]
func (ch *CommentHandler) LoadComments(ctx *gin.Context, req_ req.LoadCommentsReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ch.cs.LoadComments(ctx, req_.Id, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}
