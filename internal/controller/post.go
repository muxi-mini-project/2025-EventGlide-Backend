package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/service"
	"github.com/raiki02/EG/pkg/ginx"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type PostController struct {
	ps *service.PostService
	l  *zap.Logger
}

func NewPostController(ps *service.PostService, l *zap.Logger) *PostController {
	return &PostController{
		ps: ps,
		l:  l.Named("post/controller"),
	}
}

// @Tags Post
// @Summary 获取所有帖子
// @Produce json
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/all [get]
func (pc *PostController) GetAllPost(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := pc.ps.GetAllPost(ctx) // todo: claims 传入 studentId
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
func (pc *PostController) CreatePost(ctx *gin.Context, req_ req.CreatePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := pc.ps.CreatePost(ctx, &req_, claims.Subject)
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
func (pc *PostController) FindPostByName(ctx *gin.Context, req_ req.FindPostReq) (resp.Resp, error) {
	posts, err := pc.ps.FindPostByName(ctx, req_.Name)
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
func (pc *PostController) DeletePost(ctx *gin.Context, req_ req.DeletePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	if err := pc.ps.DeletePost(ctx, &req_, claims.Subject); err != nil {
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
func (pc *PostController) CreateDraft(ctx *gin.Context, req_ req.CreatePostReq, claims jwt.RegisteredClaims) (resp.Resp, error) {
	res, err := pc.ps.CreateDraft(ctx, &req_, claims.Subject)
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
func (pc *PostController) LoadDraft(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	draft, err := pc.ps.LoadDraft(ctx, claims.Subject)
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
func (pr *PostController) FindPostByOwnerID(ctx *gin.Context, claims jwt.RegisteredClaims) (resp.Resp, error) {
	posts, err := pr.ps.FindPostByOwnerID(ctx, claims.Subject)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(posts)
}

// @Tags Post
// @Summary 通过用户ID查找该用户发布的帖子
// @Produce json
// @Param userId path string true "用户id"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=[]resp.ListPostsResp}
// @Router /post/user/{userId} [get]
func (pr *PostController) FindPostByUserID(ctx *gin.Context, req_ req.FindPostByUserIDReq) (resp.Resp, error) {
	posts, err := pr.ps.FindPostByOwnerID(ctx, req_.UserID)
	if err != nil {
		return ginx.ReturnError(err)
	}

	return ginx.ReturnSuccess(posts)
}

// @Tags Post
// @Summary 根据id返回帖子详情
// @Produce json
// @Param id path string true "目标id"
// @Param Authorization header string true "token"
// @Success 200 {object} resp.Resp{data=resp.ListPostsResp}
// @Router /post/{id} [get]
func (pr *PostController) FindPostByBid(ctx *gin.Context, req_ req.FindPostByBidReq) (resp.Resp, error) {
	post, err := pr.ps.FindPostByBid(ctx, req_.Id)
	if err != nil {
		return ginx.ReturnError(err)
	}
	return ginx.ReturnSuccess(post)
}
