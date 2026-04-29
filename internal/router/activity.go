package router

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/controller"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/pkg/ginx"
)

type ActRouterHdl interface {
	RegisterActRouters() error
}

type ActRouter struct {
	e   *gin.Engine
	ach *controller.ActController
	j   *middleware.Jwt
}

func NewActRouter(e *gin.Engine, ach *controller.ActController, j *middleware.Jwt) *ActRouter {
	return &ActRouter{
		e:   e,
		ach: ach,
		j:   j,
	}
}

func (ar ActRouter) RegisterActRouters() {
	act := ar.e.Group("act")
	act.Use(ar.j.WrapCheckToken())
	{
		act.POST("/create", ginx.WrapRequestWithClaims(ar.ach.NewAct))
		act.POST("/draft", ginx.WrapRequestWithClaims(ar.ach.NewDraft))
		act.GET("/load", ginx.WrapWithClaims(ar.ach.LoadDraft))
		act.POST("/name", ginx.WrapRequest(ar.ach.FindActByName))
		act.POST("/date", ginx.WrapRequest(ar.ach.FindActByDate))
		act.POST("/search", ginx.WrapRequest(ar.ach.FindActBySearches))
		act.GET("/own", ginx.WrapWithClaims(ar.ach.FindActByOwnerID))
		act.GET("/all", ginx.WrapWithClaims(ar.ach.ListAllActs))
		act.GET("/:id", ginx.WrapRequest(ar.ach.FindActByBid))
	}
}
