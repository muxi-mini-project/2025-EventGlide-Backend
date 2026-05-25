package converter

import (
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
)

func CreatePostFromReq(r *req.CreatePostReq, studentID string) *model.Post {
	return &model.Post{
		Bid:       tools.GenUUID(),
		CreatedAt: time.Now(),
		StudentID: studentID,
		Title:     r.Title,
		Introduce: r.Introduce,
		ShowImg:   tools.SliceToString(r.ShowImg),
	}
}

func CreatePostDraftFromReq(r *req.CreatePostReq, studentID string) *model.PostDraft {
	return &model.PostDraft{
		Bid:       tools.GenUUID(),
		CreatedAt: time.Now(),
		StudentID: studentID,
		Title:     r.Title,
		Introduce: r.Introduce,
		ShowImg:   tools.SliceToString(r.ShowImg),
	}
}

func ToLoadPostDraftResp(d model.PostDraft) resp.LoadPostDraftResp {
	return resp.LoadPostDraftResp{
		Bid:       d.Bid,
		Title:     d.Title,
		Introduce: d.Introduce,
		ShowImg:   tools.StringToSlice(d.ShowImg),
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

func ToCreatePostResp(d model.PostDetail) resp.CreatePostResp {
	post := d.Post
	res := resp.CreatePostResp{
		Bid:         post.Bid,
		StudentID:   d.Author.StudentID,
		PublishTime: tools.ParseTime(post.CreatedAt),
		Title:       post.Title,
		Introduce:   post.Introduce,
		ShowImg:     tools.StringToSlice(post.ShowImg),
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
		Bid:         d.Bid,
		StudentID:   author.StudentID,
		PublishTime: tools.ParseTime(d.CreatedAt),
		Title:       d.Title,
		Introduce:   d.Introduce,
		ShowImg:     tools.StringToSlice(d.ShowImg),
	}
	res.UserInfo.StudentID = author.StudentID
	res.UserInfo.Avatar = author.Avatar
	res.UserInfo.Username = author.Name
	res.UserInfo.School = author.School
	return res
}
