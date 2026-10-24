package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

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
}

func (r *Handler) Run() (func(context.Context), error) {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r.e.Handler(),
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	return func(parent context.Context) {
		// HTTP 排空必须有界：从关闭总预算里最多取 5s，超时则强制断开残留连接。
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown: %v", err)
			_ = srv.Close()
		}
	}, nil
}

func (r *Handler) RegisterSwagger() {
	r.e.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
