package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/ginx"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type PostHandler struct {
	ps *service.PostService
	l  *zap.Logger
}

func NewPostHandler(e *gin.Engine, ps *service.PostService, j *middleware.Jwt, l *zap.Logger) *PostHandler {
	p := &PostHandler{
		ps: ps,
		l:  l.Named("post/handler"),
	}
	p.RegisterPostHandlers(e, j.WrapCheckToken())

	return p
}

func (ph *PostHandler) RegisterPostHandlers(e *gin.Engine, handlerFunc gin.HandlerFunc) {
	post := e.Group("/post")
	post.Use(handlerFunc)
	{
		post.GET("/all", ginx.WrapWithClaims(ph.GetAllPost))
		post.POST("/create", ginx.WrapRequestWithClaims(ph.CreatePost))
		post.POST("/find", ginx.WrapRequest(ph.FindPostByName))
		post.POST("/draft", ginx.WrapRequestWithClaims(ph.CreateDraft))
		post.POST("/delete", ginx.WrapRequestWithClaims(ph.DeletePost))
		post.GET("/load", ginx.WrapWithClaims(ph.LoadDraft))
		post.GET("/own", ginx.WrapWithClaims(ph.FindPostByOwnerID))
	}
}

// @Tags Post
// @Summary 获取所有帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/all [get]
func (ph *PostHandler) GetAllPost(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := ph.ps.GetAllPost(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(posts)
}

// @Tags Post
// @Summary 创建帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body req.CreatePostReq true "帖子"
// @Success 200 {object} resp.Resp{}
// @Router /post/create [post]
func (ph *PostHandler) CreatePost(ctx *gin.Context, req_ req.CreatePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ph.ps.CreatePost(ctx, &req_, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Post
// @Summary 通过帖子名查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param name body req.FindPostReq true "帖子名"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/find [post]
func (ph *PostHandler) FindPostByName(ctx *gin.Context, req_ req.FindPostReq) (resp.Resp, error) {
	posts, err := ph.ps.FindPostByName(ctx, req_.Name)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(posts)
}

// @Tags Post
// @Summary 删除帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body req.DeletePostReq true "帖子"
// @Success 200 {object} resp.Resp
// @Router /post/delete [post]
func (ph *PostHandler) DeletePost(ctx *gin.Context, req_ req.DeletePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ph.ps.DeletePost(ctx, &req_, claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(nil)
}

// @Tags Post
// @Summary 创建草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body req.CreatePostDraftReq true "草稿"
// @Success 200 {object} resp.Resp{data=req.CreatePostReq}
// @Router /post/draft [post]
func (ph *PostHandler) CreateDraft(ctx *gin.Context, req_ req.CreatePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := ph.ps.CreateDraft(ctx, &req_, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Post
// @Summary 加载草稿
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.LoadPostDraftResp}
// @Router /post/load [get]
func (ph *PostHandler) LoadDraft(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	draft, err := ph.ps.LoadDraft(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	res := resp.LoadPostDraftResp{
		Bid:       draft.Bid,
		Title:     draft.Title,
		Introduce: draft.Introduce,
		ShowImg:   tools.StringToSlice(draft.ShowImg),
		StudentID: draft.StudentID,
		CreatedAt: tools.ParseTime(draft.CreatedAt),
	}

	return ginx.ReturnSuccess(res)
}

// @Tags Post
// @Summary 通过用户ID查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/own [get]
func (ph *PostHandler) FindPostByOwnerID(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := ph.ps.FindPostByOwnerID(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(posts)
}
