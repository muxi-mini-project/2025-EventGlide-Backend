package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type FeedServiceHdl interface {
	GetTotalCnt(ctx *gin.Context, sid string) (resp.BriefFeedResp, error)
	GetFeedList(ctx *gin.Context, sid string) (resp.FeedResp, error)
	SubsribeTopics(ctx *gin.Context)
	GetLikeFeed(ctx *gin.Context, sid string) ([]resp.FeedLikeResp, error)
	GetCollectFeed(ctx *gin.Context, sid string) ([]resp.FeedCollectResp, error)
	GetCommentFeed(ctx *gin.Context, sid string) ([]resp.FeedCommentResp, error)
	GetAtFeed(ctx *gin.Context, sid string) ([]resp.FeedAtResp, error)
	GetAuditorFeedList(ctx *gin.Context, sid string) ([]resp.FeedInvitationResp, error)
}

type FeedService struct {
	fd *dao.FeedDao
	mq mq.MQHdl
	ud *dao.UserDao
	l  *zap.Logger
}

func NewFeedService(fd *dao.FeedDao, mq mq.MQHdl, ud *dao.UserDao, l *zap.Logger) *FeedService {
	fs := &FeedService{
		fd: fd,
		mq: mq,
		ud: ud,
		l:  l.Named("feed/service"),
	}
	fs.ConsumeFeedStream()
	return fs
}

func (fs *FeedService) ReadFeedDetail(ctx *gin.Context, sid, bid string) error {
	return fs.fd.ReadFeedDetail(ctx, sid, bid)
}

func (fs *FeedService) ReadAllFeed(ctx *gin.Context, sid string) error {
	return fs.fd.ReadAllFeed(ctx, sid)
}

func (fs *FeedService) GetTotalCnt(ctx *gin.Context, sid string) (resp.BriefFeedResp, error) {

	ints, err := fs.fd.GetTotalCnt(ctx, sid)
	if err != nil {
		fs.l.Error("Get All Events Failed", zap.Error(err))
		return resp.BriefFeedResp{}, err
	}
	return resp.BriefFeedResp{
		LikeAndCollect: ints[0],
		CommentAndAt:   ints[1],
		Total:          ints[2],
	}, nil

}

func (fs *FeedService) GetFeedList(ctx *gin.Context, sid string) (resp.FeedResp, error) {
	l, err1 := fs.GetLikeFeed(ctx, sid)
	c, err2 := fs.GetCollectFeed(ctx, sid)
	cm, err3 := fs.GetCommentFeed(ctx, sid)
	a, err4 := fs.GetAtFeed(ctx, sid)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		fs.l.Error("Get Feed List Failed", zap.Error(err1), zap.Error(err2), zap.Error(err3), zap.Error(err4))
		return resp.FeedResp{}, errors.New("get feed list error")
	}
	return resp.FeedResp{
		Likes:    l,
		Ats:      a,
		Comments: cm,
		Collects: c,
	}, nil
}

func (fs *FeedService) ConsumeFeedStream() {
	go func() {
		ctx := context.Background()
		lastIDKey := "feed_last_id"

		for {
			msgs, err := fs.mq.Consume(ctx, "feed_stream", lastIDKey, 15, 30*time.Second)
			if err != nil {
				fs.l.Error("Failed to read feed stream", zap.Error(err))
				time.Sleep(time.Second)
				continue
			}

			if len(msgs) == 0 {
				continue
			}

			for _, msg := range msgs {
				data, ok := msg.Values["data"].(string)
				if !ok {
					fs.l.Warn("Message data is not string", zap.Any("msg", msg))
					continue
				}

				var feed model.Feed
				if err := json.Unmarshal([]byte(data), &feed); err != nil {
					fs.l.Error("Failed to unmarshal feed", zap.Error(err))
					continue
				}

				feed.CreatedAt = time.Now()
				feed.Status = "未读"

				if err := fs.fd.CreateFeed(ctx, &feed); err != nil {
					fs.l.Error("Failed to consume feed", zap.Error(err), zap.String("msgID", msg.ID))
				} else {
					fs.l.Info("Feed processed", zap.Any("feed", feed))
				}
			}
		}
	}()
}

func (fs *FeedService) GetLikeFeed(ctx *gin.Context, sid string) ([]resp.FeedLikeResp, error) {
	likes, err := fs.fd.GetLikeFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Like Feed List Failed", zap.Error(err))
		return nil, err
	}
	var res []resp.FeedLikeResp
	for _, v := range likes {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentId)
		if err != nil {
			fs.l.Error("Get User Info when get like feed Failed", zap.Error(err))
			return nil, err
		}
		if sid == user.StudentID {
			continue // 不显示自己的点赞
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.TargetBid, v.Object)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get like feed Failed", zap.Error(err))
		}
		res = append(res, resp.FeedLikeResp{
			Userinfo: resp.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetBid:   v.TargetBid,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetCollectFeed(ctx *gin.Context, sid string) ([]resp.FeedCollectResp, error) {
	collects, err := fs.fd.GetCollectFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Collect Feed List Failed", zap.Error(err))
		return nil, err
	}
	var res []resp.FeedCollectResp
	for _, v := range collects {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentId)
		if err != nil {
			fs.l.Error("Get User Info when get collect feed Failed", zap.Error(err))
			return nil, err
		}
		if sid == user.StudentID {
			continue // 不显示自己的收藏
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.TargetBid, v.Object)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get collect feed Failed", zap.Error(err))
		}
		res = append(res, resp.FeedCollectResp{
			Userinfo: resp.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetBid:   v.TargetBid,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetCommentFeed(ctx *gin.Context, sid string) ([]resp.FeedCommentResp, error) {
	comments, err := fs.fd.GetCommentFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Comment Feed List Failed", zap.Error(err))
		return nil, err
	}
	var res []resp.FeedCommentResp
	for _, v := range comments {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentId)
		if err != nil {
			fs.l.Error("Get User Info when get comment feed Failed", zap.Error(err))
			return nil, err
		}
		if sid == user.StudentID {
			continue // 不显示评论自己的评论
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.TargetBid, v.Object)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get comment feed Failed", zap.Error(err))
		}
		res = append(res, resp.FeedCommentResp{
			Userinfo: resp.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetBid:   v.TargetBid,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetAtFeed(ctx *gin.Context, sid string) ([]resp.FeedAtResp, error) {
	ats, err := fs.fd.GetAtFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get At Feed List Failed", zap.Error(err))
		return nil, err
	}
	var res []resp.FeedAtResp
	for _, v := range ats {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentId)
		if err != nil {
			fs.l.Error("Get User Info when get at feed Failed", zap.Error(err))
			return nil, err
		}
		if sid == user.StudentID {
			continue // 不显示自己的@ 自己回复
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.TargetBid, v.Object)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get at feed Failed", zap.Error(err))
		}
		res = append(res, resp.FeedAtResp{
			Userinfo: resp.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetBid:   v.TargetBid,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetAuditorFeedList(ctx *gin.Context, sid string) (resp.FeedResp, error) {
	invites, err := fs.fd.GetAuditorFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Auditor Feed List Failed", zap.Error(err))
		return resp.FeedResp{}, err
	}
	var res []resp.FeedInvitationResp
	for _, v := range invites {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentId)
		if err != nil {
			fs.l.Error("Get User Info when get auditor feed Failed", zap.Error(err))
			return resp.FeedResp{}, err
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.Bid, "activity")
		if err != nil {
			fs.l.Error("Get Picture From Obj when get auditor feed Failed", zap.Error(err))
		}
		res = append(res, resp.FeedInvitationResp{
			Userinfo: resp.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Message: processMsg(&model.Feed{
				Action: "invitation",
			}, v.StudentName),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetBid:   v.Bid,
			Status:      v.Stance,
			FirstPic:    getFirstPic(pics),
		})
	}
	return resp.FeedResp{Invitations: res}, nil
}

func processMsg(f *model.Feed, name string) string {
	switch f.Action {
	case "like":
		switch f.Object {
		case "post":
			return name + "赞了你的帖子"
		case "comment":
			return name + "赞了你的评论"
		case "activity":
			return name + "赞了你的活动"
		}
	case "collect":
		switch f.Object {
		case "post":
			return name + "收藏了你的帖子"
		case "activity":
			return name + "收藏了你的活动"
		}
	case "comment":
		switch f.Object {
		case "post":
			return name + "评论了你的帖子"
		case "comment":
			return name + "评论了你的评论"
		case "activity":
			return name + "评论了你的活动"
		}
	case "at":
		switch f.Object {
		case "comment":
			return name + "在评论中@了你"
		}
	case "invitation":
		return name + "邀请你批准活动发布"
	}
	return "消息加载中......"
}

func getFirstPic(pics string) string {
	// http://xxx,http://yyy
	if strings.Contains(pics, ",http") {
		return strings.Split(pics, ",")[0]
	}

	// http://xxx
	if pics != "" {
		return pics
	}

	// no pic
	return ""
}
