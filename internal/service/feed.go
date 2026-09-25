package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/safe"
	"github.com/raiki02/EG/tools"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ FeedServiceHdl = &FeedService{}

type FeedServiceHdl interface {
	ReadFeedDetail(ctx context.Context, sid string, id int64) error
	ReadAllFeed(ctx context.Context, sid string) error
	GetTotalCnt(ctx context.Context, sid string) (model.BriefFeedDetail, error)
	GetFeedList(ctx context.Context, sid string) (model.FeedDetail, error)
	GetLikeFeed(ctx context.Context, sid string) ([]model.FeedLikeDetail, error)
	ConsumeFeedStream()
	GetCollectFeed(ctx context.Context, sid string) ([]model.FeedCollectDetail, error)
	GetCommentFeed(ctx context.Context, sid string) ([]model.FeedCommentDetail, error)
	GetAtFeed(ctx context.Context, sid string) ([]model.FeedAtDetail, error)
	GetAuditorFeedList(ctx context.Context, sid string) (model.FeedDetail, error)
}

type FeedService struct {
	fd *dao.FeedDao
	mq mq.MQHdl
	ud *repo.UserRepo
	l  *zap.Logger
}

var feedConsumerLifecycle struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

const (
	feedStream       = "feed_stream"
	feedGroup        = "feed_consumers"
	feedDLQKey       = "feed_dlq"
	feedBatch        = int64(15)
	feedBlockFor     = 30 * time.Second
	feedRecoverEvery = 30 * time.Second
	feedRecoverIdle  = 2 * time.Minute
	feedMaxRetry     = 3
)

func NewFeedService(fd *dao.FeedDao, mq mq.MQHdl, ud *repo.UserRepo, l *logger.LoggerSet) *FeedService {
	fs := &FeedService{
		fd: fd,
		mq: mq,
		ud: ud,
		l:  l.Feed.Named("service"),
	}
	fs.ConsumeFeedStream()
	return fs
}

func (fs *FeedService) ReadFeedDetail(ctx context.Context, sid string, id int64) error {
	return fs.fd.ReadFeedDetail(ctx, sid, id)
}

func (fs *FeedService) ReadAllFeed(ctx context.Context, sid string) error {
	return fs.fd.ReadAllFeed(ctx, sid)
}

func (fs *FeedService) GetTotalCnt(ctx context.Context, sid string) (model.BriefFeedDetail, error) {
	ints, err := fs.fd.GetTotalCnt(ctx, sid)
	if err != nil {
		fs.l.Error("Get All Events Failed", zap.Error(err))
		return model.BriefFeedDetail{}, errs.ErrInternal.Wrap(err)
	}
	return model.BriefFeedDetail{
		LikeAndCollect: ints.LikeAndCollect,
		CommentAndAt:   ints.CommentAndAt,
		Total:          ints.Total,
	}, nil
}

func (fs *FeedService) GetFeedList(ctx context.Context, sid string) (model.FeedDetail, error) {
	l, err1 := fs.GetLikeFeed(ctx, sid)
	c, err2 := fs.GetCollectFeed(ctx, sid)
	cm, err3 := fs.GetCommentFeed(ctx, sid)
	a, err4 := fs.GetAtFeed(ctx, sid)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		fs.l.Error("Get Feed List Failed", zap.Error(err1), zap.Error(err2), zap.Error(err3), zap.Error(err4))
		return model.FeedDetail{}, errs.ErrInternal.Wrap(err1)
	}
	return model.FeedDetail{
		Likes:    l,
		Ats:      a,
		Comments: cm,
		Collects: c,
	}, nil
}

func (fs *FeedService) ConsumeFeedStream() {
	feedConsumerLifecycle.mu.Lock()
	if feedConsumerLifecycle.cancel != nil {
		feedConsumerLifecycle.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	feedConsumerLifecycle.cancel = cancel
	feedConsumerLifecycle.mu.Unlock()

	host, err := os.Hostname()
	if err != nil {
		host = "unknown-host"
	}
	consumer := fmt.Sprintf("%s-%d", host, time.Now().UnixNano())

	if err := fs.mq.EnsureConsumerGroup(ctx, feedStream, feedGroup); err != nil {
		fs.l.Error("Failed to ensure feed consumer group", zap.Error(err))
		return
	}

	safe.Go(fs.l, "feed-consumer", func() { fs.consumeFeedLoop(ctx, consumer) })
	safe.Go(fs.l, "feed-consumer.recover", func() { fs.recoverFeedLoop(ctx, consumer) })
}

// consumeFeedLoop 持续读取 feed_stream 的新消息。
func (fs *FeedService) consumeFeedLoop(ctx context.Context, consumer string) {
	for {
		if ctx.Err() != nil {
			return
		}
		// 逐轮隔离：单轮（含读流）panic 不终止整个消费循环。
		safe.Run(fs.l, "feed-consumer.consume", func() { fs.consumeFeedOnce(ctx, consumer) })
	}
}

func (fs *FeedService) consumeFeedOnce(ctx context.Context, consumer string) {
	msgs, err := fs.mq.ConsumeGroup(ctx, feedStream, feedGroup, consumer, feedBatch, feedBlockFor)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fs.l.Info("Feed consumer stopped")
			return
		}
		fs.l.Error("Failed to read feed stream", zap.Error(err))
		time.Sleep(time.Second)
		return
	}
	for _, msg := range msgs {
		safe.Run(fs.l, "feed-consumer.processMessage", func() { fs.processFeedMessage(ctx, msg) })
	}
}

// recoverFeedLoop 定期认领 feed_stream 中空闲超时、未 ACK 的消息重投，
// 弥补消费失败后消息滞留在 PEL 而无人捞取。
func (fs *FeedService) recoverFeedLoop(ctx context.Context, consumer string) {
	ticker := time.NewTicker(feedRecoverEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			safe.Run(fs.l, "feed-consumer.recover", func() { fs.recoverFeedPending(ctx, consumer) })
		}
	}
}

func (fs *FeedService) recoverFeedPending(ctx context.Context, consumer string) {
	// XAUTOCLAIM 的起始游标必须是合法 stream ID，"0-0" 表示从最早开始；
	// 空串会被 Redis 拒绝（ERR Invalid stream ID）。
	start := "0-0"
	for {
		msgs, nextStart, err := fs.mq.AutoClaim(ctx, feedStream, feedGroup, consumer, feedRecoverIdle, start)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return
			}
			fs.l.Error("AutoClaim feed failed", zap.Error(err))
			return
		}
		for _, msg := range msgs {
			safe.Run(fs.l, "feed-consumer.recoverMessage", func() { fs.recoverFeedMessage(ctx, msg) })
		}
		start = nextStart
		if nextStart == "" || nextStart == "0-0" {
			return
		}
	}
}

// recoverFeedMessage 处理重投的消息：超过重试上限转死信；否则按正常逻辑再处理。
func (fs *FeedService) recoverFeedMessage(ctx context.Context, msg redis.XMessage) {
	pending, err := fs.mq.ListPendingExt(ctx, feedStream, feedGroup, 0, msg.ID, msg.ID, 1)
	if err != nil {
		fs.l.Error("ListPendingExt feed failed", zap.Error(err), zap.String("msgID", msg.ID))
		return
	}
	if len(pending) > 0 && pending[0].RetryCount >= feedMaxRetry {
		fs.l.Warn("Feed message delivery count exceeded, moving to DLQ", zap.String("msgID", msg.ID))
		// 先写 DLQ 再 ACK：若先 ACK 后 Publish 失败，消息会从 PEL 与 DLQ 双双丢失。
		if data, ok := msg.Values["data"].(string); ok {
			var feed model.Feed
			if json.Unmarshal([]byte(data), &feed) == nil {
				if err := fs.mq.Publish(ctx, feedDLQKey, feed); err != nil {
					fs.l.Error("Failed to publish feed to DLQ, leaving for retry", zap.Error(err), zap.String("msgID", msg.ID))
					return
				}
			}
		}
		fs.ackFeed(ctx, msg.ID)
		return
	}
	fs.processFeedMessage(ctx, msg)
}

// processFeedMessage 处理单条消息；入库失败不 ACK，留在 PEL 由 recoverLoop 重投。
func (fs *FeedService) processFeedMessage(ctx context.Context, msg redis.XMessage) {
	data, ok := msg.Values["data"].(string)
	if !ok {
		fs.l.Warn("Feed message data is not string", zap.Any("msg", msg))
		fs.ackFeed(ctx, msg.ID)
		return
	}
	var feed model.Feed
	if err := json.Unmarshal([]byte(data), &feed); err != nil {
		fs.l.Error("Failed to unmarshal feed", zap.Error(err))
		fs.ackFeed(ctx, msg.ID)
		return
	}

	feed.CreatedAt = time.Now()
	feed.Status = "未读"
	if feed.Object == SubjectComment {
		rootID, rootType, resolveErr := fs.fd.ResolveRootMetaByCommentID(ctx, feed.TargetId)
		if resolveErr != nil {
			fs.l.Warn("Failed to resolve feed root id", zap.Error(resolveErr), zap.Int64("targetId", feed.TargetId))
		} else {
			feed.RootID = rootID
			feed.RootType = rootType
		}
	}

	if err := fs.fd.CreateFeed(ctx, &feed); err != nil {
		// 入库失败：不 ACK，留在 PEL，由 recoverLoop 重投；达上限后转 DLQ。
		fs.l.Error("Failed to consume feed, left in PEL for retry", zap.Error(err), zap.String("msgID", msg.ID))
		return
	}
	fs.l.Info("Feed processed", zap.Any("feed", feed))
	fs.ackFeed(ctx, msg.ID)
}

func (fs *FeedService) ackFeed(ctx context.Context, msgID string) {
	if err := fs.mq.Ack(ctx, feedStream, feedGroup, msgID); err != nil {
		fs.l.Error("Failed to ack feed message", zap.Error(err), zap.String("msgID", msgID))
	}
}

func (fs *FeedService) GetLikeFeed(ctx context.Context, sid string) ([]model.FeedLikeDetail, error) {
	likes, err := fs.fd.GetLikeFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Like Feed List Failed", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	var res []model.FeedLikeDetail
	for _, v := range likes {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentID)
		if err != nil {
			fs.l.Error("Get User Info when get like feed Failed", zap.Error(err))
			return nil, errs.ErrUserNotFound.Wrap(err)
		}
		resolvedRootID, resolvedRootType := fs.resolveRootMeta(ctx, v)
		pics, err := fs.loadFeedPicture(ctx, v, resolvedRootID, resolvedRootType)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get like feed Failed", zap.Error(err))
		}
		res = append(res, model.FeedLikeDetail{
			Userinfo: model.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetId:    v.TargetId,
			RootID:      resolvedRootID,
			RootType:    resolvedRootType,
			Subject:     v.Object,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetCollectFeed(ctx context.Context, sid string) ([]model.FeedCollectDetail, error) {
	collects, err := fs.fd.GetCollectFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Collect Feed List Failed", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	var res []model.FeedCollectDetail
	for _, v := range collects {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentID)
		if err != nil {
			fs.l.Error("Get User Info when get collect feed Failed", zap.Error(err))
			return nil, errs.ErrUserNotFound.Wrap(err)
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.TargetId, v.Object)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get collect feed Failed", zap.Error(err))
		}
		res = append(res, model.FeedCollectDetail{
			Userinfo: model.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetId:    v.TargetId,
			RootID:      v.RootID,
			RootType:    v.RootType,
			Subject:     v.Object,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetCommentFeed(ctx context.Context, sid string) ([]model.FeedCommentDetail, error) {
	comments, err := fs.fd.GetCommentFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Comment Feed List Failed", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	var res []model.FeedCommentDetail
	for _, v := range comments {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentID)
		if err != nil {
			fs.l.Error("Get User Info when get comment feed Failed", zap.Error(err))
			return nil, errs.ErrUserNotFound.Wrap(err)
		}
		resolvedRootID, resolvedRootType := fs.resolveRootMeta(ctx, v)
		pics, err := fs.loadFeedPicture(ctx, v, resolvedRootID, resolvedRootType)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get comment feed Failed", zap.Error(err))
		}
		res = append(res, model.FeedCommentDetail{
			Userinfo: model.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetId:    v.TargetId,
			RootID:      resolvedRootID,
			RootType:    resolvedRootType,
			Subject:     v.Object,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetAtFeed(ctx context.Context, sid string) ([]model.FeedAtDetail, error) {
	ats, err := fs.fd.GetAtFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get At Feed List Failed", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	var res []model.FeedAtDetail
	for _, v := range ats {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentID)
		if err != nil {
			fs.l.Error("Get User Info when get at feed Failed", zap.Error(err))
			return nil, errs.ErrUserNotFound.Wrap(err)
		}
		resolvedRootID, resolvedRootType := fs.resolveRootMeta(ctx, v)
		pics, err := fs.loadFeedPicture(ctx, v, resolvedRootID, resolvedRootType)
		if err != nil {
			fs.l.Error("Get Picture From Obj when get at feed Failed", zap.Error(err))
		}
		res = append(res, model.FeedAtDetail{
			Userinfo: model.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Id:          v.Id,
			Message:     processMsg(v, user.Name),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetId:    v.TargetId,
			RootID:      resolvedRootID,
			RootType:    resolvedRootType,
			Subject:     v.Object,
			Status:      v.Status,
			FirstPic:    getFirstPic(pics),
		})
	}
	return res, nil
}

func (fs *FeedService) GetAuditorFeedList(ctx context.Context, sid string) (model.FeedDetail, error) {
	invites, err := fs.fd.GetAuditorFeed(ctx, sid)
	if err != nil {
		fs.l.Error("Get Auditor Feed List Failed", zap.Error(err))
		return model.FeedDetail{}, errs.ErrInternal.Wrap(err)
	}
	var res []model.FeedInvitationDetail
	for _, v := range invites {
		user, err := fs.ud.GetUserInfo(ctx, v.StudentId)
		if err != nil {
			fs.l.Error("Get User Info when get auditor feed Failed", zap.Error(err))
			return model.FeedDetail{}, errs.ErrUserNotFound.Wrap(err)
		}
		pics, err := fs.fd.GetPictureFromObj(ctx, v.ActivityId, "activity")
		if err != nil {
			fs.l.Error("Get Picture From Obj when get auditor feed Failed", zap.Error(err))
		}
		res = append(res, model.FeedInvitationDetail{
			Userinfo: model.UserInfo{
				StudentID: user.StudentID,
				Avatar:    user.Avatar,
				Username:  user.Name,
			},
			Message: processMsg(&model.Feed{
				Action: "invitation",
			}, string(v.StudentName)),
			PublishedAt: tools.ParseTime(v.CreatedAt),
			TargetId:    v.ActivityId,
			RootID:      0,
			RootType:    "",
			Subject:     SubjectActivity,
			Status:      v.Stance,
			FirstPic:    getFirstPic(pics),
		})
	}
	return model.FeedDetail{Invitations: res}, nil
}

func (fs *FeedService) resolveRootMeta(ctx context.Context, f *model.Feed) (int64, string) {
	if f.Object != SubjectComment {
		return 0, ""
	}

	rootID := f.RootID
	rootType := f.RootType
	if rootID == 0 || rootType == "" {
		resolvedRootID, resolvedRootType, err := fs.fd.ResolveRootMetaByCommentID(ctx, f.TargetId)
		if err != nil {
			fs.l.Warn("Resolve root id for feed subject failed", zap.Error(err), zap.Int64("feedID", f.Id), zap.Int64("targetId", f.TargetId))
			return 0, ""
		}
		rootID = resolvedRootID
		rootType = resolvedRootType
	}
	return rootID, rootType
}

func (fs *FeedService) loadFeedPicture(ctx context.Context, f *model.Feed, resolvedRootID int64, resolvedRootType string) (string, error) {
	if f.Object == SubjectComment && resolvedRootID != 0 && resolvedRootType != "" {
		return fs.fd.GetPictureFromObj(ctx, resolvedRootID, resolvedRootType)
	}
	return fs.fd.GetPictureFromObj(ctx, f.TargetId, f.Object)
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
