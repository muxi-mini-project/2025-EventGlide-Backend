package converter

import (
	"testing"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
)

// TestPostToAuditReqMapsRow 送审请求必须完整还原已落库帖子的标题、正文与图片，
// 供后台 worker 异步送审（图片顺序即用户提交顺序）。
func TestPostToAuditReqMapsRow(t *testing.T) {
	post := &model.Post{
		Title:     "社团招新",
		Introduce: "本周日在操场",
		Images: []model.Image{
			{Url: "https://img.example/a.png", Width: 800, Height: 600},
			{Url: "https://img.example/b.png", Width: 400, Height: 300},
		},
	}

	got := PostToAuditReq(post)

	if got.Title != post.Title {
		t.Errorf("Title = %q, want %q", got.Title, post.Title)
	}
	if got.Introduce != post.Introduce {
		t.Errorf("Introduce = %q, want %q", got.Introduce, post.Introduce)
	}
	if len(got.ShowImg) != 2 || got.ShowImg[0].Url != "https://img.example/a.png" || got.ShowImg[1].Url != "https://img.example/b.png" {
		t.Errorf("ShowImg = %v, want the two image urls in order", got.ShowImg)
	}
	if got.ShowImg[0].Width != 800 || got.ShowImg[0].Height != 600 {
		t.Errorf("ShowImg[0] = %+v, want width 800 height 600", got.ShowImg[0])
	}
}

// TestCreatePostFromReqDefaultsToPublicPass 新建帖子一律公开且不送审（is_checking=pass），
// 私密只能由作者事后在详情页切换。
func TestCreatePostFromReqDefaultsToPublicPass(t *testing.T) {
	got := CreatePostFromReq(&req.CreatePostReq{Title: "招新", Introduce: "正文"}, "S1")

	if got.Visibility != model.PostVisibilityPublic {
		t.Errorf("Visibility = %q, want %q", got.Visibility, model.PostVisibilityPublic)
	}
	if got.IsChecking != "pass" {
		t.Errorf("IsChecking = %q, want pass (posts are not audited)", got.IsChecking)
	}
}

// TestToListPostRespCarriesVisibility 列表/详情响应需带 visibility，供前端渲染左下角开关。
func TestToListPostRespCarriesVisibility(t *testing.T) {
	got := ToListPostResp(model.PostDetail{
		Post: model.Post{Id: 1, Visibility: model.PostVisibilityPrivate},
	})

	if got.Visibility != model.PostVisibilityPrivate {
		t.Errorf("Visibility = %q, want %q", got.Visibility, model.PostVisibilityPrivate)
	}
}
