package router

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/controller"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/pkg/ginx"
)

type PostRouterHdl interface {
	RegisterPostRouters()
}

type PostRouter struct {
	e   *gin.Engine
	j   *middleware.Jwt
	pch *controller.PostController
}

func NewPostRouter(e *gin.Engine, pch *controller.PostController, j *middleware.Jwt) *PostRouter {
	return &PostRouter{
		e:   e,
		pch: pch,
		j:   j,
	}
}

func (pr *PostRouter) RegisterPostRouters() {
	post := pr.e.Group("/post")
	post.Use(pr.j.WrapCheckToken())
	{
		post.GET("/all", ginx.WrapWithClaims(pr.pch.GetAllPost))
		post.POST("/create", ginx.WrapRequestWithClaims(pr.pch.CreatePost))
		post.POST("/find", ginx.WrapRequest(pr.pch.FindPostByName))
		post.POST("/draft", ginx.WrapRequestWithClaims(pr.pch.CreateDraft))
		post.POST("/delete", ginx.WrapRequestWithClaims(pr.pch.DeletePost))
		post.GET("/load", ginx.WrapWithClaims(pr.pch.LoadDraft))
		post.GET("/own", ginx.WrapWithClaims(pr.pch.FindPostByOwnerID))
		post.GET("/user/:userId", ginx.WrapRequest(pr.pch.FindPostByUserID))
		post.GET("/:id", ginx.WrapRequest(pr.pch.FindPostByBid))
	}
}
