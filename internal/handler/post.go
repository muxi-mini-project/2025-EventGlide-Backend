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
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/utils"
	"go.uber.org/zap"
)

type PostHandler struct {
	ps *service.PostService
	l  *zap.Logger
}

func NewPostHandler(e *gin.Engine, ps *service.PostService, j *middleware.Jwt, l *logger.LoggerSet) *PostHandler {
	p := &PostHandler{
		ps: ps,
		l:  l.Post.Named("handler"),
	}
	p.RegisterPostHandlers(e, j.WrapCheckToken())

	return p
}

func (ph *PostHandler) RegisterPostHandlers(e *gin.Engine, handlerFunc gin.HandlerFunc) {
	post := e.Group("/post")
	post.Use(handlerFunc)
	{
		post.POST("/all", ginx.WrapRequestWithClaims(ph.GetAllPost))
		post.POST("/create", ginx.WrapRequestWithClaims(ph.CreatePost))
		post.POST("/find", ginx.WrapRequestWithClaims(ph.FindPostByName))
		post.POST("/draft", ginx.WrapRequestWithClaims(ph.CreateDraft))
		post.POST("/delete", ginx.WrapRequestWithClaims(ph.DeletePost))
		post.GET("/load", ginx.WrapWithClaims(ph.LoadDraft))
		post.POST("/own", ginx.WrapRequestWithClaims(ph.FindPostByOwnerID))
		post.POST("/student/:studentId", ginx.WrapRequestWithClaims(ph.FindPostByStudentID))
		post.GET("/:id", ginx.WrapRequestWithClaims(ph.FindPostById))
	}
}

// GetAllPost
// @Tags Post
// @Summary 获取所有帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param req body req.ListAllPostsReq true "分页请求"
// @Success 200 {object} resp.Resp{data=resp.PaginatedListPostsResp}
// @Router /post/all [post]
func (ph *PostHandler) GetAllPost(ctx *gin.Context, req_ req.ListAllPostsReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	req_.Page, req_.Limit = utils.IndexValid(req_.Page, req_.Limit)
	paginated, err := ph.ps.GetAllPost(ctx, req_.Page, req_.Limit)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichForSearcher(ctx, paginated.Posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToPaginatedListPostsResp(paginated.Total, paginated.Page, paginated.Limit, details))
}

// CreatePost
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
	loadedPost, err := ph.ps.FindPostById(ctx, post.Id)
	if err != nil {
		return ginx.ReturnError(err)
	}
	detail := ph.ps.EnrichOneForSearcher(ctx, &loadedPost, claims.Subject)
	return ginx.ReturnSuccess(converter.ToCreatePostResp(detail))
}

// FindPostByName
// @Tags Post
// @Summary 通过帖子名查找帖子
// @Produce json
// @Param Authorization header string true "token"
// @Param req body req.FindPostReq true "帖子名"
// @Success 200 {object} resp.Resp{data=resp.PaginatedListPostsResp}
// @Router /post/find [post]
func (ph *PostHandler) FindPostByName(ctx *gin.Context, req_ req.FindPostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	req_.Page, req_.Limit = utils.IndexValid(req_.Page, req_.Limit)
	paginated, err := ph.ps.FindPostByName(ctx, req_.Name, req_.Page, req_.Limit)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichForSearcher(ctx, paginated.Posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToPaginatedListPostsResp(paginated.Total, paginated.Page, paginated.Limit, details))
}

// DeletePost
// @Tags Post
// @Summary 删除帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param post body req.DeletePostReq true "帖子"
// @Success 200 {object} resp.Resp
// @Router /post/delete [post]
func (ph *PostHandler) DeletePost(ctx *gin.Context, req_ req.DeletePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := ph.ps.DeletePost(ctx, int64(req_.TargetID), claims.Subject); err != nil {
		return ginx.ReturnError(err)
	}
	return ginx.ReturnSuccess(nil)
}

// CreateDraft
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

// LoadDraft
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

// FindPostByOwnerID
// @Tags Post
// @Summary 通过用户ID查找帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param req body req.FindPostByOwnerIDReq true "分页请求"
// @Success 200 {object} resp.Resp{data=resp.PaginatedListPostsResp}
// @Router /post/own [post]
func (ph *PostHandler) FindPostByOwnerID(ctx *gin.Context, req_ req.FindPostByOwnerIDReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	req_.Page, req_.Limit = utils.IndexValid(req_.Page, req_.Limit)
	paginated, err := ph.ps.FindPostByOwnerID(ctx, claims.Subject, req_.Page, req_.Limit)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichForSearcher(ctx, paginated.Posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToPaginatedListPostsResp(paginated.Total, paginated.Page, paginated.Limit, details))
}

// FindPostByStudentID
// @Tags Post
// @Summary 通过学号获取帖子
// @Produce json
// @Accept json
// @Param Authorization header string true "token"
// @Param studentId path string true "学号"
// @Param req body req.FindPostByStudentIDReq true "分页请求"
// @Success 200 {object} resp.Resp{data=resp.PaginatedListPostsResp}
// @Router /post/student/{studentId} [post]
func (ph *PostHandler) FindPostByStudentID(ctx *gin.Context, req_ req.FindPostByStudentIDReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	req_.Page, req_.Limit = utils.IndexValid(req_.Page, req_.Limit)
	paginated, err := ph.ps.FindPostByOwnerID(ctx, req_.StudentID, req_.Page, req_.Limit)
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichForSearcher(ctx, paginated.Posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToPaginatedListPostsResp(paginated.Total, paginated.Page, paginated.Limit, details))
}

// FindPostById
// @Tags Post
// @Summary 通过bid返回帖子详情
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.ListPostsResp}
// @Router /post/{id} [get]
func (ph *PostHandler) FindPostById(ctx *gin.Context, req_ req.FindPostByIdReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := ph.ps.FindPostById(ctx, int64(req_.Id))
	if err != nil {
		return ginx.ReturnError(err)
	}
	details := ph.ps.EnrichOneForSearcher(ctx, &posts, claims.Subject)
	return ginx.ReturnSuccess(converter.ToListPostResp(details))
}
