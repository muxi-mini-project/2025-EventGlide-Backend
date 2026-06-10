package converter

import (
	"strings"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/utils"
)

func ToGetTotalCntResp(detail model.BriefFeedDetail) resp.BriefFeedResp {
	return resp.BriefFeedResp{
		LikeAndCollect: detail.LikeAndCollect,
		CommentAndAt:   detail.CommentAndAt,
		Total:          detail.Total,
	}
}

func ToGetFeedListResp(detail model.FeedDetail) resp.FeedResp {
	return resp.FeedResp{
		Likes:       ToGetLikeFeedResp(detail.Likes),
		Collects:    ToGetCollectFeedResp(detail.Collects),
		Comments:    ToGetCommentFeedResp(detail.Comments),
		Ats:         ToGetAtFeedResp(detail.Ats),
		Invitations: ToGetAuditorFeedListResp(detail.Invitations),
	}
}

func ToGetLikeFeedResp(feeds []model.FeedLikeDetail) []resp.FeedLikeResp {
	var res []resp.FeedLikeResp

	for _, v := range feeds {
		res = append(res, resp.FeedLikeResp{
			Id:          utils.SnowflakeID(v.Id),
			Message:     v.Message,
			PublishedAt: v.PublishedAt,
			TargetId:    utils.SnowflakeID(v.TargetId),
			RootID:      utils.SnowflakeID(v.RootID),
			RootType:    v.RootType,
			Subject:     v.Subject,
			Status:      v.Status,
			FirstPic:    v.FirstPic,
			Userinfo: resp.FeedUserInfo{
				StudentID: v.Userinfo.StudentID,
				Avatar:    v.Userinfo.Avatar,
				Username:  v.Userinfo.Username,
			},
		})
	}

	return res
}

func ToGetCollectFeedResp(feeds []model.FeedCollectDetail) []resp.FeedCollectResp {
	var res []resp.FeedCollectResp

	for _, v := range feeds {
		res = append(res, resp.FeedCollectResp{
			Id:          utils.SnowflakeID(v.Id),
			Message:     v.Message,
			PublishedAt: v.PublishedAt,
			TargetId:    utils.SnowflakeID(v.TargetId),
			RootID:      utils.SnowflakeID(v.RootID),
			RootType:    v.RootType,
			Subject:     v.Subject,
			Status:      v.Status,
			FirstPic:    v.FirstPic,
			Userinfo: resp.FeedUserInfo{
				StudentID: v.Userinfo.StudentID,
				Avatar:    v.Userinfo.Avatar,
				Username:  v.Userinfo.Username,
			},
		})
	}

	return res
}

func ToGetCommentFeedResp(feeds []model.FeedCommentDetail) []resp.FeedCommentResp {
	var res []resp.FeedCommentResp

	for _, v := range feeds {
		res = append(res, resp.FeedCommentResp{
			Id:          utils.SnowflakeID(v.Id),
			Message:     v.Message,
			PublishedAt: v.PublishedAt,
			TargetId:    utils.SnowflakeID(v.TargetId),
			RootID:      utils.SnowflakeID(v.RootID),
			RootType:    v.RootType,
			Subject:     v.Subject,
			Status:      v.Status,
			FirstPic:    v.FirstPic,
			Userinfo: resp.FeedUserInfo{
				StudentID: v.Userinfo.StudentID,
				Avatar:    v.Userinfo.Avatar,
				Username:  v.Userinfo.Username,
			},
		})
	}

	return res
}

func ToGetAtFeedResp(feeds []model.FeedAtDetail) []resp.FeedAtResp {
	var res []resp.FeedAtResp

	for _, v := range feeds {
		res = append(res, resp.FeedAtResp{
			Id:          utils.SnowflakeID(v.Id),
			Message:     v.Message,
			PublishedAt: v.PublishedAt,
			TargetId:    utils.SnowflakeID(v.TargetId),
			RootID:      utils.SnowflakeID(v.RootID),
			RootType:    v.RootType,
			Subject:     v.Subject,
			Status:      v.Status,
			FirstPic:    v.FirstPic,
			Userinfo: resp.FeedUserInfo{
				StudentID: v.Userinfo.StudentID,
				Avatar:    v.Userinfo.Avatar,
				Username:  v.Userinfo.Username,
			},
		})
	}

	return res
}

func ToGetAuditorFeedListResp(feeds []model.FeedInvitationDetail) []resp.FeedInvitationResp {
	var res []resp.FeedInvitationResp

	for _, v := range feeds {
		res = append(res, resp.FeedInvitationResp{
			Id:          utils.SnowflakeID(v.Id),
			Message:     v.Message,
			PublishedAt: v.PublishedAt,
			TargetId:    utils.SnowflakeID(v.TargetId),
			RootID:      utils.SnowflakeID(v.RootID),
			RootType:    v.RootType,
			Subject:     v.Subject,
			Status:      v.Status,
			FirstPic:    v.FirstPic,
			Userinfo: resp.FeedUserInfo{
				StudentID: v.Userinfo.StudentID,
				Avatar:    v.Userinfo.Avatar,
				Username:  v.Userinfo.Username,
			},
		})
	}

	return res
}

func GetFirstPic(pics string) string {
	if strings.Contains(pics, ",http") {
		return strings.Split(pics, ",")[0]
	}

	if pics != "" {
		return pics
	}

	return ""
}

func FeedFromInteractionReq(r *req.InteractionReq, action string, studentID string, receiver string) model.Feed {
	return model.Feed{
		TargetId: int64(r.TargetID),
		Object:   r.Subject,
		StudentID: studentID,
		Action:   action,
		Receiver: receiver,
	}
}