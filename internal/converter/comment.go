package converter

import (
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
)

func CommentFromReq(r req.CreateCommentReq, studentID string) *model.Comment {
	return &model.Comment{
		StudentID: studentID,
		Content:   r.Content,
		ParentID:  r.ParentID,
		CreatedAt: time.Now(),
		Bid:       tools.GenUUID(),
		Position:  "华中师范大学",
		Subject:   r.Subject,
	}
}

func ToCommentResp(d model.CommentDetail) resp.CommentResp {
	cmt := d.Comment
	res := resp.CommentResp{
		Bid:           cmt.Bid,
		CommentedTime: tools.ParseTime(cmt.CreatedAt),
		CommentedPos:  cmt.Position,
		Content:       cmt.Content,
		LikeNum:       cmt.LikeNum,
		ReplyNum:      cmt.ReplyNum,
		ParentID:      cmt.ParentID,
		RootID:        cmt.RootID,
	}
	if d.IsLike {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}
	res.Creator.StudentID = cmt.StudentID
	res.Creator.Username = cmt.CreatorName
	res.Creator.Avatar = cmt.CreatorAvatar
	for _, reply := range d.Replies {
		res.Reply = append(res.Reply, ToReplyResp(reply))
	}
	return res
}

func ToCommentResps(details []model.CommentDetail) []resp.CommentResp {
	res := make([]resp.CommentResp, 0, len(details))
	for _, d := range details {
		res = append(res, ToCommentResp(d))
	}
	return res
}

func ToReplyResp(d model.ReplyDetail) resp.ReplyResp {
	cmt := d.Comment
	res := resp.ReplyResp{
		Bid:            cmt.Bid,
		ReplyContent:   cmt.Content,
		ReplyTime:      tools.ParseTime(cmt.CreatedAt),
		ReplyPos:       cmt.Position,
		ParentID:       cmt.ParentID,
		RootID:         cmt.RootID,
		ParentUserName: cmt.ReplyToUserName,
		LikeNum:        cmt.LikeNum,
		ReplyNum:       cmt.ReplyNum,
	}
	if d.IsLike {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}
	res.ReplyCreator.StudentID = cmt.StudentID
	res.ReplyCreator.Username = cmt.CreatorName
	res.ReplyCreator.Avatar = cmt.CreatorAvatar
	return res
}
