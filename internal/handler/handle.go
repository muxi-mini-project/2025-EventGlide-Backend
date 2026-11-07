package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/internal/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Handler struct {
	e    *gin.Engine
	cors *middleware.Cors
	uh   *UserHandler
	ah   *ActHandler
	ph   *PostHandler
	ch   *CommentHandler
	fh   *FeedHandler
	ih   *InteractionHandler

	cba *CallbackAuditorHandler
}

func NewHandler(e *gin.Engine, cors *middleware.Cors, uh *UserHandler, ah *ActHandler, ph *PostHandler, ch *CommentHandler, fh *FeedHandler, ih *InteractionHandler, cba *CallbackAuditorHandler) *Handler {
	return &Handler{
		e:    e,
		cors: cors,
		uh:   uh,
		ah:   ah,
		ph:   ph,
		ch:   ch,
		fh:   fh,
		ih:   ih,
		cba:  cba,
	}
}

func (r *Handler) RegisterHandlers() {
	r.cors.HandleCors()
	r.RegisterSwagger()
	r.e.GET("/healthz", r.healthz)
}

// healthz 供容器 HEALTHCHECK 探测进程与 HTTP 可用性；不查 DB/Redis，避免依赖抖动放大故障。
func (r *Handler) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (r *Handler) Run() (error, func()) {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r.e.Handler(),
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	return nil, func() {
		if err := srv.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}
}

func (r *Handler) RegisterSwagger() {
	r.e.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
