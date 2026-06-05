package converter

import (
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
)

func CreatePostFromReq(r *req.CreatePostReq, studentID string) *model.Post {
	id := tools.MustGenerateID()
	return &model.Post{
		Id:        id,
		CreatedAt: time.Now(),
		StudentID: studentID,
		Title:     r.Title,
		Introduce: r.Introduce,
		Images:    ImagesFromUrls(r.ShowImg, id, "post"),
	}
}

func CreatePostDraftFromReq(r *req.CreatePostReq, studentID string) *model.PostDraft {
	id := tools.MustGenerateID()
	return &model.PostDraft{
		Id:        id,
		CreatedAt: time.Now(),
		StudentID: studentID,
		Title:     r.Title,
		Introduce: r.Introduce,
		Images:    ImagesFromUrls(r.ShowImg, id, "post_draft"),
	}
}

func ToLoadPostDraftResp(d model.PostDraft) resp.LoadPostDraftResp {
	return resp.LoadPostDraftResp{
		Id:        d.Id,
		Title:     d.Title,
		Introduce: d.Introduce,
		ShowImg:   ImagesToUrls(d.Images),
		StudentID: d.StudentID,
		CreatedAt: tools.ParseTime(d.CreatedAt),
	}
}

func ToListPostsResp(details []model.PostDetail) []resp.ListPostsResp {
	res := make([]resp.ListPostsResp, 0, len(details))
	for _, d := range details {
		res = append(res, ToListPostResp(d))
	}
	return res
}

func ToPaginatedListPostsResp(total int64, page, limit int, details []model.PostDetail) resp.PaginatedListPostsResp {
	return resp.PaginatedListPostsResp{
		Total:   total,
		Page:    page,
		Limit:   limit,
		Details: ToListPostsResp(details),
	}
}

func ToListPostResp(d model.PostDetail) resp.ListPostsResp {
	post := d.Post
	var res resp.ListPostsResp

	if d.IsCollect {
		res.IsCollect = "true"
	} else {
		res.IsCollect = "false"
	}
	if d.IsLike {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}

	res.UserInfo.School = d.Author.School
	res.UserInfo.Username = d.Author.Name
	res.UserInfo.Avatar = d.Author.Avatar
	res.UserInfo.StudentID = d.Author.StudentID
	res.PublishTime = tools.ParseTime(post.CreatedAt)
	res.Title = post.Title
	res.Introduce = post.Introduce
	res.IsChecking = post.IsChecking
	res.ShowImg = ImagesToUrls(post.Images)
	res.LikeNum = post.LikeNum
	res.CommentNum = post.CommentNum
	res.CollectNum = post.CollectNum
	res.Id = post.Id

	return res
}

func ToCreatePostResp(d model.PostDetail) resp.CreatePostResp {
	post := d.Post
	res := resp.CreatePostResp{
		Id:          post.Id,
		StudentID:   d.Author.StudentID,
		PublishTime: tools.ParseTime(post.CreatedAt),
		Title:       post.Title,
		Introduce:   post.Introduce,
		ShowImg:     ImagesToUrls(post.Images),
		IsChecking:  post.IsChecking,
	}
	res.UserInfo.StudentID = d.Author.StudentID
	res.UserInfo.Avatar = d.Author.Avatar
	res.UserInfo.Username = d.Author.Name
	res.UserInfo.School = d.Author.School
	return res
}

func ToCreatePostRespFromDraft(d model.PostDraft, author model.UserBrief) resp.CreatePostResp {
	res := resp.CreatePostResp{
		Id:          d.Id,
		StudentID:   author.StudentID,
		PublishTime: tools.ParseTime(d.CreatedAt),
		Title:       d.Title,
		Introduce:    d.Introduce,
		ShowImg:      ImagesToUrls(d.Images),
	}
	res.UserInfo.StudentID = author.StudentID
	res.UserInfo.Avatar = author.Avatar
	res.UserInfo.Username = author.Name
	res.UserInfo.School = author.School
	return res
}