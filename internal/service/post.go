package service

import (
	"context"
	"strings"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"go.uber.org/zap"
)

var _ PostServiceHdl = &PostService{}

type PostServiceHdl interface {
	GetAllPost(context.Context) ([]model.Post, error)
	CreatePost(context.Context, *model.Post, *req.AuditWrapper) error
	FindPostByName(context.Context, string) ([]model.Post, error)
	DeletePost(context.Context, string, string) error
	CreateDraft(context.Context, *model.PostDraft) error
	LoadDraft(context.Context, string) (model.PostDraft, error)
	FindPostByOwnerID(context.Context, string) ([]model.Post, error)
	FindPostByBid(context.Context, string) (model.Post, error)
	EnrichForSearcher(context.Context, []model.Post, string) []model.PostDetail
	EnrichOneForSearcher(context.Context, *model.Post, string) model.PostDetail
	AuthorBrief(context.Context, string) model.UserBrief
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

func (ps *PostService) GetAllPost(c context.Context) ([]model.Post, error) {
	return ps.pdh.GetAllPost(c)
}

func (ps *PostService) CreatePost(c context.Context, post *model.Post, aw *req.AuditWrapper) error {
	form, err := ps.aud.CreateAuditorForm(c, post.Bid, "", SubjectPost)
	if err != nil {
		ps.l.Error("Failed to create auditor form", zap.Error(err), zap.String("bid", post.Bid))
		return err
	}

	err = ps.aud.UploadForm(c, aw, form.Id)
	if err != nil {
		ps.l.Error("Failed to upload form", zap.Error(err), zap.String("bid", post.Bid), zap.Uint("formID", form.Id))
		return err
	}

	err = ps.pdh.CreatePost(c, post)
	if err != nil {
		return err
	}
	return nil
}

func (ps *PostService) FindPostByName(c context.Context, name string) ([]model.Post, error) {
	return ps.pdh.FindPostByName(c, name)
}

func (ps *PostService) DeletePost(c context.Context, bid, studentID string) error {
	return ps.pdh.DeletePost(c, &model.Post{
		Bid:       bid,
		StudentID: studentID,
	})
}

func (ps *PostService) CreateDraft(c context.Context, draft *model.PostDraft) error {
	return ps.pdh.CreateDraft(c, draft)
}

func (ps *PostService) LoadDraft(c context.Context, sid string) (model.PostDraft, error) {
	return ps.pdh.LoadDraft(c, sid)
}

func (ps *PostService) FindPostByOwnerID(c context.Context, studentID string) ([]model.Post, error) {
	return ps.pdh.FindPostByOwnerID(c, studentID)
}

func (ps *PostService) FindPostByBid(c context.Context, bid string) (model.Post, error) {
	return ps.pdh.FindPostByBid(c, bid)
}

func (ps *PostService) EnrichForSearcher(c context.Context, posts []model.Post, viewerID string) []model.PostDetail {
	details := make([]model.PostDetail, 0, len(posts))
	for i := range posts {
		details = append(details, ps.enrichOne(c, &posts[i], viewerID))
	}
	return details
}

func (ps *PostService) EnrichOneForSearcher(c context.Context, post *model.Post, viewerID string) model.PostDetail {
	return ps.enrichOne(c, post, viewerID)
}

func (ps *PostService) AuthorBrief(c context.Context, studentID string) model.UserBrief {
	user := ps.ud.FindUserByID(c, studentID)
	return model.UserBrief{
		StudentID: user.StudentID,
		Name:      user.Name,
		Avatar:    user.Avatar,
		School:    user.School,
	}
}

func (ps *PostService) enrichOne(c context.Context, post *model.Post, viewerID string) model.PostDetail {
	searcher := ps.ud.FindUserByID(c, viewerID)
	author := ps.ud.FindUserByID(c, post.StudentID)

	return model.PostDetail{
		Post: *post,
		Author: model.UserBrief{
			StudentID: author.StudentID,
			Name:      author.Name,
			Avatar:    author.Avatar,
			School:    author.School,
		},
		IsLike:    strings.Contains(searcher.LikePost, post.Bid),
		IsCollect: strings.Contains(searcher.CollectPost, post.Bid),
	}
}
