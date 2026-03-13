package router

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/controller"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/pkg/ginx"
)

type CommentRouterHdl interface {
	RegisterCommentRouter()
}

type CommentRouter struct {
	cch *controller.CommentController
	e   *gin.Engine
	j   *middleware.Jwt
}

func NewCommentRouter(cch *controller.CommentController, e *gin.Engine, j *middleware.Jwt) *CommentRouter {
	return &CommentRouter{
		cch: cch,
		e:   e,
		j:   j,
	}
}

func (cr *CommentRouter) RegisterCommentRouter() {
	cmt := cr.e.Group("/comment")
	cmt.Use(cr.j.WrapCheckToken())
	{
		cmt.POST("/create", ginx.WrapRequestWithClaims(cr.cch.CreateComment))
		cmt.POST("/delete", ginx.WrapRequestWithClaims(cr.cch.DeleteComment))
		cmt.POST("/answer", ginx.WrapRequestWithClaims(cr.cch.AnswerComment))
		cmt.GET("/load/:id", ginx.WrapRequestWithClaims(cr.cch.LoadComments))
	}
}
