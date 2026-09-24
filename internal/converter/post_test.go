package converter

import (
	"testing"

	"github.com/raiki02/EG/internal/model"
)

// TestPostToAuditReqMapsRow 送审请求必须完整还原已落库帖子的标题、正文与图片，
// 供后台 worker 异步送审（图片顺序即用户提交顺序）。
func TestPostToAuditReqMapsRow(t *testing.T) {
	post := &model.Post{
		Title:     "社团招新",
		Introduce: "本周日在操场",
		Images: []model.Image{
			{Url: "https://img.example/a.png"},
			{Url: "https://img.example/b.png"},
		},
	}

	got := PostToAuditReq(post)

	if got.Title != post.Title {
		t.Errorf("Title = %q, want %q", got.Title, post.Title)
	}
	if got.Introduce != post.Introduce {
		t.Errorf("Introduce = %q, want %q", got.Introduce, post.Introduce)
	}
	if len(got.ShowImg) != 2 || got.ShowImg[0] != "https://img.example/a.png" || got.ShowImg[1] != "https://img.example/b.png" {
		t.Errorf("ShowImg = %v, want the two image urls in order", got.ShowImg)
	}
}
