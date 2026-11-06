package converter

import (
	"testing"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
)

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
