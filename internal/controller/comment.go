package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/ginx"
	"go.uber.org/zap"
)

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
func (cc *CommentController) CreateComment(ctx *gin.Context, req_ req.CreateCommentReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := cc.cs.CreateComment(ctx, req_, claims.Subject)
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
func (cc *CommentController) AnswerComment(ctx *gin.Context, req_ req.CreateCommentReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := cc.cs.AnswerComment(ctx, req_, claims.Subject)
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
func (cc *CommentController) DeleteComment(ctx *gin.Context, req_ req.DeleteCommentReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	err := cc.cs.DeleteComment(ctx, req_.TargetID, claims.Subject)
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
func (cc *CommentController) LoadComments(ctx *gin.Context, req_ req.LoadCommentsReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := cc.cs.LoadComments(ctx, req_.Id, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}
