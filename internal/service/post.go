package service

import (
	"context"
	"strings"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

var _ PostServiceHdl = &PostService{}

type PostServiceHdl interface {
	GetAllPost(context.Context, int, int) (*model.PaginatedPosts, error)
	CreatePost(context.Context, *model.Post, *req.AuditWrapper) error
	FindPostByName(context.Context, string, int, int) (*model.PaginatedPosts, error)
	DeletePost(context.Context, string, string) error
	CreateDraft(context.Context, *model.PostDraft) error
	LoadDraft(context.Context, string) (model.PostDraft, error)
	FindPostByOwnerID(context.Context, string, int, int) (*model.PaginatedPosts, error)
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

func NewPostService(pdh *repo.PostRepo, ud *repo.UserRepo, aud AuditorService, l *logger.LoggerSet) *PostService {
	return &PostService{
		pdh: pdh,
		ud:  ud,
		aud: aud,
		l:   l.Post.Named("service"),
	}
}

func (ps *PostService) GetAllPost(c context.Context, page, limit int) (*model.PaginatedPosts, error) {
	return ps.pdh.GetAllPost(c, page, limit)
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

func (ps *PostService) FindPostByName(c context.Context, name string, page, limit int) (*model.PaginatedPosts, error) {
	return ps.pdh.FindPostByName(c, name, page, limit)
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

func (ps *PostService) FindPostByOwnerID(c context.Context, studentID string, page, limit int) (*model.PaginatedPosts, error) {
	return ps.pdh.FindPostByOwnerID(c, studentID, page, limit)
}

func (ps *PostService) FindPostByBid(c context.Context, bid string) (model.Post, error) {
	return ps.pdh.FindPostByBid(c, bid)
}

func (ps *PostService) EnrichForSearcher(c context.Context, posts []model.Post, viewerID string) []model.PostDetail {
	studentIDs := make([]string, 0, len(posts)+1)
	studentIDs = append(studentIDs, viewerID)
	for _, post := range posts {
		studentIDs = append(studentIDs, post.StudentID)
	}
	usersMap, _ := ps.ud.GetUsersByIDs(c, studentIDs)
	searcher := usersMap[viewerID]

	details := make([]model.PostDetail, 0, len(posts))
	for i := range posts {
		post := &posts[i]
		author := usersMap[post.StudentID]
		if author == nil {
			author = &model.User{}
		}
		details = append(details, model.PostDetail{
			Post: *post,
			Author: model.UserBrief{
				StudentID: author.StudentID,
				Name:      author.Name,
				Avatar:    author.Avatar,
				School:    author.School,
			},
			IsLike:    strings.Contains(searcher.LikePost, post.Bid),
			IsCollect: strings.Contains(searcher.CollectPost, post.Bid),
		})
	}
	return details
}

func (ps *PostService) EnrichOneForSearcher(c context.Context, post *model.Post, viewerID string) model.PostDetail {
	return ps.enrichOne(c, post, viewerID)
}

func (ps *PostService) AuthorBrief(c context.Context, studentID string) model.UserBrief {
	usersMap, _ := ps.ud.GetUsersByIDs(c, []string{studentID})
	if len(usersMap) == 0 {
		return model.UserBrief{}
	}
	user := usersMap[studentID]
	if user == nil {
		return model.UserBrief{}
	}
	return model.UserBrief{
		StudentID: user.StudentID,
		Name:      user.Name,
		Avatar:    user.Avatar,
		School:    user.School,
	}
}

func (ps *PostService) enrichOne(c context.Context, post *model.Post, viewerID string) model.PostDetail {
	usersMap, _ := ps.ud.GetUsersByIDs(c, []string{viewerID, post.StudentID})
	searcher := usersMap[viewerID]
	author := usersMap[post.StudentID]
	if searcher == nil {
		searcher = &model.User{}
	}
	if author == nil {
		author = &model.User{}
	}

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
