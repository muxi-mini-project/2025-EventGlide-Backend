package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/ginx"
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
		post.POST("/find", ginx.WrapRequestWithClaims(ph.FindPostByName))
		post.POST("/draft", ginx.WrapRequestWithClaims(ph.CreateDraft))
		post.POST("/delete", ginx.WrapRequestWithClaims(ph.DeletePost))
		post.GET("/load", ginx.WrapWithClaims(ph.LoadDraft))
		post.GET("/own", ginx.WrapWithClaims(ph.FindPostByOwnerID))
		post.GET("/:id", ginx.WrapRequestWithClaims(ph.FindPostByBid))
	}
}

// @Tags Post
// @Summary 获取所有帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/all [get]
func (ph *PostHandler) GetAllPost(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := ph.ps.GetAllPost(ctx)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichForSearcher(ctx, posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToListPostsResp(details))
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
	post := converter.CreatePostFromReq(&req_, claims.Subject)
	aw := &req.AuditWrapper{
		Subject:   service.SubjectPost,
		StudentId: claims.Subject,
		CpostReq:  &req_,
	}
	if err := ph.ps.CreatePost(ctx, post, aw); err != nil {
		return ginx.ReturnError(err)
	}
	detail := ph.ps.EnrichOneForSearcher(ctx, post, claims.Subject)
	return ginx.ReturnSuccess(converter.ToCreatePostResp(detail))
}

// @Tags Post
// @Summary 通过帖子名查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param name body req.FindPostReq true "帖子名"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/find [post]
func (ph *PostHandler) FindPostByName(ctx *gin.Context, req_ req.FindPostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := ph.ps.FindPostByName(ctx, req_.Name)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichForSearcher(ctx, posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToListPostsResp(details))
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
	if err := ph.ps.DeletePost(ctx, req_.TargetID, claims.Subject); err != nil {
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
	draft := converter.CreatePostDraftFromReq(&req_, claims.Subject)
	if err := ph.ps.CreateDraft(ctx, draft); err != nil {
		return ginx.ReturnError(err)
	}
	author := ph.ps.AuthorBrief(ctx, draft.StudentID)
	return ginx.ReturnSuccess(converter.ToCreatePostRespFromDraft(*draft, author))
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
	return ginx.ReturnSuccess(converter.ToLoadPostDraftResp(draft))
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
	details := ph.ps.EnrichForSearcher(ctx, posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToListPostsResp(details))
}

func (ph *PostHandler) FindPostByBid(ctx *gin.Context, req_ req.FindPostByBidReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := ph.ps.FindPostByBid(ctx, req_.Id)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichOneForSearcher(ctx, &posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToListPostResp(details))
}
