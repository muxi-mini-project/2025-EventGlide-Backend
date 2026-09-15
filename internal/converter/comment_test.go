package converter

import (
	"testing"

	"github.com/raiki02/EG/internal/model"
)

// TestToCommentRespPrefersLiveCreator 评论作者昵称/头像以实时查询为准，
// 改名或换头像后不应继续显示评论时的快照。
func TestToCommentRespPrefersLiveCreator(t *testing.T) {
	d := model.CommentDetail{
		Comment: model.Comment{
			StudentID:     "S20250002",
			CreatorName:   "旧名",
			CreatorAvatar: "https://old.example/a.png",
		},
		Creator: model.UserBrief{Name: "新名", Avatar: "https://new.example/a.png"},
	}

	res := ToCommentResp(d)
	if res.Creator.Username != "新名" {
		t.Errorf("Username = %q, want 新名", res.Creator.Username)
	}
	if res.Creator.Avatar != "https://new.example/a.png" {
		t.Errorf("Avatar = %q, want live avatar", res.Creator.Avatar)
	}
}

// TestToCommentRespFallsBackToSnapshot 实时用户查不到（整批查询失败或用户已注销）时回退快照，
// 避免头像/昵称变成空值。
func TestToCommentRespFallsBackToSnapshot(t *testing.T) {
	d := model.CommentDetail{
		Comment: model.Comment{
			StudentID:     "S20250002",
			CreatorName:   "快照名",
			CreatorAvatar: "https://snap.example/a.png",
		},
	}

	res := ToCommentResp(d)
	if res.Creator.Username != "快照名" {
		t.Errorf("Username = %q, want 快照名", res.Creator.Username)
	}
	if res.Creator.Avatar != "https://snap.example/a.png" {
		t.Errorf("Avatar = %q, want snapshot avatar", res.Creator.Avatar)
	}
}

// TestToReplyRespPrefersLiveCreator 回复作者同样实时优先，且只影响作者本人，
// ReplyToUserName（“回复@xxx”）保持历史快照不变。
func TestToReplyRespPrefersLiveCreator(t *testing.T) {
	d := model.ReplyDetail{
		Comment: model.Comment{
			StudentID:       "S20250003",
			CreatorName:     "旧名",
			CreatorAvatar:   "https://old.example/a.png",
			ReplyToUserName: "被回复人快照",
		},
		Creator: model.UserBrief{Name: "新名", Avatar: "https://new.example/a.png"},
	}

	res := ToReplyResp(d)
	if res.ReplyCreator.Username != "新名" {
		t.Errorf("ReplyCreator.Username = %q, want 新名", res.ReplyCreator.Username)
	}
	if res.ReplyCreator.Avatar != "https://new.example/a.png" {
		t.Errorf("ReplyCreator.Avatar = %q, want live avatar", res.ReplyCreator.Avatar)
	}
	if res.ParentUserName != "被回复人快照" {
		t.Errorf("ParentUserName = %q, want snapshot unchanged", res.ParentUserName)
	}
}

// TestToReplyRespFallsBackToSnapshot 回复作者实时查不到时回退快照。
func TestToReplyRespFallsBackToSnapshot(t *testing.T) {
	d := model.ReplyDetail{
		Comment: model.Comment{
			StudentID:     "S20250003",
			CreatorName:   "快照名",
			CreatorAvatar: "https://snap.example/a.png",
		},
	}

	res := ToReplyResp(d)
	if res.ReplyCreator.Username != "快照名" {
		t.Errorf("ReplyCreator.Username = %q, want 快照名", res.ReplyCreator.Username)
	}
	if res.ReplyCreator.Avatar != "https://snap.example/a.png" {
		t.Errorf("ReplyCreator.Avatar = %q, want snapshot avatar", res.ReplyCreator.Avatar)
	}
}
