package service

import (
	"context"
	"strings"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

var _ PostServiceHdl = &PostService{}

type PostServiceHdl interface {
	GetAllPost(context.Context, string) ([]resp.ListPostsResp, error)
	CreatePost(context.Context, *req.CreatePostReq, string) (resp.CreatePostResp, error)
	FindPostByName(context.Context, string, string) ([]resp.ListPostsResp, error)
	DeletePost(context.Context, *req.DeletePostReq, string) error
	CreateDraft(context.Context, *req.CreatePostReq, string) (resp.CreatePostResp, error)
	LoadDraft(context.Context, string) (resp.LoadPostDraftResp, error)
	FindPostByOwnerID(context.Context, string) ([]resp.ListPostsResp, error)
	FindPostByBid(context.Context, string, string) (resp.ListPostsResp, error)
}

type PostService struct {
	aud AuditorService
	pdh *repo.PostRepo
	ud  *repo.UserRepo
	l   *zap.Logger
}

func NewPostService(pdh *repo.PostRepo, ud *repo.UserRepo, l *zap.Logger, aud AuditorService) *PostService {
	return &PostService{
		pdh: pdh,
		ud:  ud,
		aud: aud,
		l:   l.Named("post/service"),
	}
}

func (ps *PostService) GetAllPost(c context.Context, studentId string) ([]resp.ListPostsResp, error) {
	posts, err := ps.pdh.GetAllPost(c)
	if err != nil {
		return nil, err
	}
	res := ps.ToListResp(c, posts, studentId)
	return res, nil
}

func (ps *PostService) CreatePost(c context.Context, r *req.CreatePostReq, studentId string) (resp.CreatePostResp, error) {
	var (
		err  error
		form *model.AuditorForm
	)
	post := toPost(r, studentId)

	form, err = ps.aud.CreateAuditorForm(c, post.Bid, "", SubjectPost)
	if err != nil {
		ps.l.Error("Failed to create auditor form", zap.Error(err), zap.String("bid", post.Bid))
		return resp.CreatePostResp{}, err
	}

	aw := &req.AuditWrapper{
		Subject:   SubjectPost,
		StudentId: studentId,
		CpostReq:  r,
	}
	err = ps.aud.UploadForm(c, aw, form.Id)
	if err != nil {
		ps.l.Error("Failed to upload form", zap.Error(err), zap.String("bid", post.Bid), zap.Uint("formID", form.Id))
		return resp.CreatePostResp{}, err
	}

	err = ps.pdh.CreatePost(c, post)
	if err != nil {
		return resp.CreatePostResp{}, err
	}

	return ps.toCreateResp(c, post), nil
}

func (ps *PostService) FindPostByName(c context.Context, name string, studentId string) ([]resp.ListPostsResp, error) {
	posts, err := ps.pdh.FindPostByName(c, name)
	if err != nil {
		return nil, err
	}
	res := ps.ToListResp(c, posts, studentId)
	return res, nil
}
func (ps *PostService) DeletePost(c context.Context, post *req.DeletePostReq, studentId string) error {
	err := ps.pdh.DeletePost(c, &model.Post{
		Bid:       post.TargetID,
		StudentID: studentId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (ps *PostService) CreateDraft(c context.Context, r *req.CreatePostReq, studentId string) (resp.CreatePostResp, error) {
	draft := toDraft(r, studentId)
	err := ps.pdh.CreateDraft(c, draft)
	if err != nil {
		return resp.CreatePostResp{}, err
	}
	return ps.toCreateResp(c, draft), nil
}

func (ps *PostService) LoadDraft(c context.Context, sid string) (resp.LoadPostDraftResp, error) {
	draft, err := ps.pdh.LoadDraft(c, sid)
	if err != nil {
		return resp.LoadPostDraftResp{}, err
	}

	res := resp.LoadPostDraftResp{
		Bid:       draft.Bid,
		Title:     draft.Title,
		Introduce: draft.Introduce,
		ShowImg:   tools.StringToSlice(draft.ShowImg),
		StudentID: draft.StudentID,
		CreatedAt: tools.ParseTime(draft.CreatedAt),
	}

	return res, nil
}

func (ps *PostService) FindPostByOwnerID(c context.Context, studentId string) ([]resp.ListPostsResp, error) {
	posts, err := ps.pdh.FindPostByOwnerID(c, studentId)
	if err != nil {
		return nil, err
	}
	res := ps.ToListResp(c, posts, studentId)
	return res, nil
}

func (ps *PostService) FindPostByBid(c context.Context, bid string, studentId string) (resp.ListPostsResp, error) {
	post, err := ps.pdh.FindPostByBid(c, bid)
	if err != nil {
		return resp.ListPostsResp{}, err
	}
	res := ps.toListPostResp(c, post, studentId)
	return res, nil
}

func (ps *PostService) ToListResp(c context.Context, posts []model.Post, studentId string) []resp.ListPostsResp {
	var res []resp.ListPostsResp
	for _, post := range posts {
		res = append(res, ps.toListPostResp(c, post, studentId))
	}
	return res
}

func (ps *PostService) toListPostResp(c context.Context, post model.Post, studentId string) resp.ListPostsResp {
	user := ps.ud.FindUserByID(c, post.StudentID)
	var res resp.ListPostsResp
	// TODO 类型断言error判断
	searcher := ps.ud.FindUserByID(c, studentId)
	if strings.Contains(searcher.CollectPost, post.Bid) {
		res.IsCollect = "true"
	} else {
		res.IsCollect = "false"
	}
	if strings.Contains(searcher.LikePost, post.Bid) {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}
	res.UserInfo.School = user.School
	res.UserInfo.Username = user.Name
	res.UserInfo.Avatar = user.Avatar
	res.UserInfo.StudentID = user.StudentID
	res.Bid = post.Bid
	res.PublishTime = tools.ParseTime(post.CreatedAt)

	res.Title = post.Title
	res.Introduce = post.Introduce
	res.IsChecking = post.IsChecking
	res.ShowImg = tools.StringToSlice(post.ShowImg)
	res.LikeNum = post.LikeNum
	res.CommentNum = post.CommentNum
	res.CollectNum = post.CollectNum
	return res
}

func toPost(r *req.CreatePostReq, studentId string) *model.Post {
	return &model.Post{
		Bid:       tools.GenUUID(),
		CreatedAt: time.Now(),

		StudentID: studentId,
		Title:     r.Title,
		Introduce: r.Introduce,
		ShowImg:   tools.SliceToString(r.ShowImg),
	}
}

func toDraft(r *req.CreatePostReq, studentId string) *model.PostDraft {
	return &model.PostDraft{
		Bid:       tools.GenUUID(),
		CreatedAt: time.Now(),
		StudentID: studentId,
		Title:     r.Title,
		Introduce: r.Introduce,
		ShowImg:   tools.SliceToString(r.ShowImg),
	}
}

func (ps *PostService) toCreateResp(c context.Context, p any) resp.CreatePostResp {
	switch p.(type) {
	case *model.Post:
		post := p.(*model.Post)
		var res resp.CreatePostResp
		user := ps.ud.FindUserByID(c, post.StudentID)
		res.UserInfo.School = user.School
		res.UserInfo.Username = user.Name
		res.UserInfo.Avatar = user.Avatar
		res.StudentID = user.StudentID
		res.UserInfo.StudentID = user.StudentID
		res.Title = post.Title
		res.Bid = post.Bid
		res.IsChecking = post.IsChecking
		res.Introduce = post.Introduce
		res.ShowImg = tools.StringToSlice(post.ShowImg)
		res.PublishTime = tools.ParseTime(post.CreatedAt)
		return res
	case *model.PostDraft:
		draft := p.(*model.PostDraft)
		var res resp.CreatePostResp
		user := ps.ud.FindUserByID(c, draft.StudentID)
		res.UserInfo.School = user.School
		res.UserInfo.Username = user.Name
		res.UserInfo.Avatar = user.Avatar
		res.UserInfo.StudentID = user.StudentID
		res.Title = draft.Title
		res.Introduce = draft.Introduce
		res.ShowImg = tools.StringToSlice(draft.ShowImg)
		res.PublishTime = tools.ParseTime(draft.CreatedAt)
		res.Bid = draft.Bid
		return res

	default:
		return resp.CreatePostResp{}
	}
}
