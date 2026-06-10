package service

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

var _ InteractionServiceHdl = &InteractionService{}

type InteractionServiceHdl interface {
	Like(c *gin.Context, r *req.InteractionReq, sid string) error
	Dislike(c *gin.Context, r *req.InteractionReq, sid string) error
	Comment(c *gin.Context, r *req.InteractionReq, sid string) error
	Collect(c *gin.Context, r *req.InteractionReq, sid string) error
	DisCollect(c *gin.Context, r *req.InteractionReq, sid string) error
	Approve(c *gin.Context, r *req.InteractionReq, studendId string) error
	Reject(c *gin.Context, r *req.InteractionReq, studendId string) error
}

type InteractionService struct {
	sg  SubjectGetter
	id  *repo.InteractionRepo
	mq  mq.MQHdl
	lfr *cache.LikeFavoriteRedis // Redis 缓存层
	l   *zap.Logger
}

func NewInteractionService(id *repo.InteractionRepo, mq mq.MQHdl, sg SubjectGetter, lfr *cache.LikeFavoriteRedis, l *logger.LoggerSet) *InteractionService {
	return &InteractionService{
		id:  id,
		sg:  sg,
		mq:  mq,
		lfr: lfr,
		l:   l.Interaction.Named("service"),
	}
}

// getUserIDByStudentID 根据学生 ID 获取用户 ID
func (is *InteractionService) getUserIDByStudentID(c context.Context, studentID string) (int64, error) {
	userID, err := is.id.GetUserIDByStudentID(c, studentID)
	if err != nil {
		is.l.Error("Failed to get user info", zap.Error(err), zap.String("studentID", studentID))
		return 0, errs.ErrInternal.Wrap(err)
	}
	return userID, nil
}

func (is *InteractionService) Like(c *gin.Context, r *req.InteractionReq, sid string) error {
	ctx := c.Request.Context()
	ap, err := is.sg.GetSubjectInfo(ctx, int64(r.TargetID), r.Subject)
	if err != nil {
		is.l.Error("Failed to get subject info", zap.Error(err), zap.Int64("targetId", int64(r.TargetID)), zap.String("subject", r.Subject))
		return errs.ErrInternal.Wrap(err)
	}

	// 获取用户 ID
	userID, err := is.getUserIDByStudentID(ctx, sid)
	if err != nil {
		return err
	}

	// 转换 subject
	var subject cache.Subject
	switch r.Subject {
	case SubjectActivity:
		subject = cache.SubjectActivity
	case SubjectPost:
		subject = cache.SubjectPost
	case SubjectComment:
		subject = cache.SubjectComment
	default:
		return errs.ErrInteractionSubjectInvalid
	}

	// Redis 点赞
	added, err := is.lfr.Like(ctx, subject, int64(r.TargetID), userID)
	if err != nil {
		is.l.Error("Redis Like failed", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}

	// 发送 MQ（如果 Redis 操作成功）
	if added {
		event := mq.InteractionEvent{
			Type:      "like",
			Action:    "add",
			Subject:   r.Subject,
			SubjectID: int64(r.TargetID),
			UserID:    userID,
			Timestamp: time.Now().Unix(),
		}
		if err := is.mq.Publish(ctx, mq.StreamKey, event); err != nil {
			// MQ 发送失败，回滚 Redis
			if _, rollbackErr := is.lfr.Unlike(ctx, subject, int64(r.TargetID), userID); rollbackErr != nil {
				is.l.Error("Rollback failed", zap.Error(rollbackErr), zap.Int64("targetId", int64(r.TargetID)))
			}
			is.l.Error("MQ Publish failed, rolled back", zap.Error(err))
			return errs.ErrInternal.Wrap(err)
		}
	}

	// 发送 feed（如果需要）
	if sid != ap.StudentID {
		jreq := converter.FeedFromInteractionReq(r, "like", sid, ap.StudentID)
		err = is.mq.Publish(ctx, "feed_stream", jreq)
		if err != nil {
			is.l.Error("Publish Like Feed Failed", zap.Error(err))
		}
	}

	return nil
}

func (is *InteractionService) Dislike(c *gin.Context, r *req.InteractionReq, sid string) error {
	userID, err := is.getUserIDByStudentID(c.Request.Context(), sid)
	if err != nil {
		return err
	}

	var subject cache.Subject
	switch r.Subject {
	case SubjectActivity:
		subject = cache.SubjectActivity
	case SubjectPost:
		subject = cache.SubjectPost
	case SubjectComment:
		subject = cache.SubjectComment
	default:
		return errs.ErrInteractionSubjectInvalid
	}

	// Redis 取消点赞
	removed, err := is.lfr.Unlike(c.Request.Context(), subject, int64(r.TargetID), userID)
	if err != nil {
		is.l.Error("Redis Unlike failed", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}

	// 发送 MQ
	if removed {
		event := mq.InteractionEvent{
			Type:      "like",
			Action:    "remove",
			Subject:   r.Subject,
			SubjectID: int64(r.TargetID),
			UserID:    userID,
			Timestamp: time.Now().Unix(),
		}
		if err := is.mq.Publish(c.Request.Context(), mq.StreamKey, event); err != nil {
			// MQ 发送失败，回滚 Redis
			if _, rollbackErr := is.lfr.Like(c.Request.Context(), subject, int64(r.TargetID), userID); rollbackErr != nil {
				is.l.Error("Rollback failed", zap.Error(rollbackErr), zap.Int64("targetId", int64(r.TargetID)))
			}
			is.l.Error("MQ Publish failed, rolled back", zap.Error(err))
			return errs.ErrInternal.Wrap(err)
		}
	}

	return nil
}

func (is *InteractionService) Comment(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.sg.GetSubjectInfo(c.Request.Context(), int64(r.TargetID), r.Subject)
	if err != nil {
		is.l.Error("Failed to get subject info", zap.Error(err), zap.Int64("targetId", int64(r.TargetID)), zap.String("subject", r.Subject))
		return errs.ErrInternal.Wrap(err)
	}
	if sid != ap.StudentID {
		jreq := converter.FeedFromInteractionReq(r, SubjectComment, sid, ap.StudentID)
		err = is.mq.Publish(c.Request.Context(), "feed_stream", jreq)
		if err != nil {
			is.l.Error("Publish Comment Feed Failed", zap.Error(err), zap.Any("feed", jreq))
		} else {
			is.l.Info("Publish Comment Feed Success", zap.Any("feed", jreq))
		}
	}

	switch r.Subject {
	case SubjectActivity:
		return is.id.CommentActivity(c, sid, int64(r.TargetID))
	case SubjectPost:
		return is.id.CommentPost(c, sid, int64(r.TargetID))
	}
	return nil
}

func (is *InteractionService) Collect(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.sg.GetSubjectInfo(c.Request.Context(), int64(r.TargetID), r.Subject)
	if err != nil {
		is.l.Error("Failed to get subject info", zap.Error(err), zap.Int64("targetId", int64(r.TargetID)), zap.String("subject", r.Subject))
		return errs.ErrInternal.Wrap(err)
	}

	// 获取用户 ID
	userID, err := is.getUserIDByStudentID(c.Request.Context(), sid)
	if err != nil {
		return err
	}

	// 转换 subject
	var subject cache.Subject
	switch r.Subject {
	case SubjectActivity:
		subject = cache.SubjectActivity
	case SubjectPost:
		subject = cache.SubjectPost
	default:
		return errs.ErrInteractionSubjectInvalid
	}

	// Redis 收藏
	added, err := is.lfr.Collect(c.Request.Context(), subject, int64(r.TargetID), userID)
	if err != nil {
		is.l.Error("Redis Collect failed", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}

	// 发送 MQ（如果 Redis 操作成功）
	if added {
		event := mq.InteractionEvent{
			Type:      "collect",
			Action:    "add",
			Subject:   r.Subject,
			SubjectID: int64(r.TargetID),
			UserID:    userID,
			Timestamp: time.Now().Unix(),
		}
		if err := is.mq.Publish(c.Request.Context(), mq.StreamKey, event); err != nil {
			// MQ 发送失败，回滚 Redis
			if _, rollbackErr := is.lfr.Uncollect(c.Request.Context(), subject, int64(r.TargetID), userID); rollbackErr != nil {
				is.l.Error("Rollback failed", zap.Error(rollbackErr), zap.Int64("targetId", int64(r.TargetID)))
			}
			is.l.Error("MQ Publish failed, rolled back", zap.Error(err))
			return errs.ErrInternal.Wrap(err)
		}
	}

	// 发送 feed（如果需要）
	if sid != ap.StudentID {
		jreq := converter.FeedFromInteractionReq(r, "collect", sid, ap.StudentID)
		err = is.mq.Publish(c.Request.Context(), "feed_stream", jreq)
		if err != nil {
			is.l.Error("Publish Collect Feed Failed", zap.Error(err), zap.Any("feed", jreq))
		} else {
			is.l.Info("Publish Collect Feed Success", zap.Any("feed", jreq))
		}
	}

	return nil
}

func (is *InteractionService) DisCollect(c *gin.Context, r *req.InteractionReq, sid string) error {
	userID, err := is.getUserIDByStudentID(c.Request.Context(), sid)
	if err != nil {
		return err
	}

	var subject cache.Subject
	switch r.Subject {
	case SubjectActivity:
		subject = cache.SubjectActivity
	case SubjectPost:
		subject = cache.SubjectPost
	default:
		return errs.ErrInteractionSubjectInvalid
	}

	// Redis 取消收藏
	removed, err := is.lfr.Uncollect(c.Request.Context(), subject, int64(r.TargetID), userID)
	if err != nil {
		is.l.Error("Redis Uncollect failed", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}

	// 发送 MQ
	if removed {
		event := mq.InteractionEvent{
			Type:      "collect",
			Action:    "remove",
			Subject:   r.Subject,
			SubjectID: int64(r.TargetID),
			UserID:    userID,
			Timestamp: time.Now().Unix(),
		}
		if err := is.mq.Publish(c.Request.Context(), mq.StreamKey, event); err != nil {
			// MQ 发送失败，回滚 Redis
			if _, rollbackErr := is.lfr.Collect(c.Request.Context(), subject, int64(r.TargetID), userID); rollbackErr != nil {
				is.l.Error("Rollback failed", zap.Error(rollbackErr), zap.Int64("targetId", int64(r.TargetID)))
			}
			is.l.Error("MQ Publish failed, rolled back", zap.Error(err))
			return errs.ErrInternal.Wrap(err)
		}
	}

	return nil
}

func (is *InteractionService) Approve(c *gin.Context, r *req.InteractionReq, studendId string) error {
	return is.id.ApproveActivity(c, studendId, int64(r.TargetID))
}

func (is *InteractionService) Reject(c *gin.Context, r *req.InteractionReq, studendId string) error {
	return is.id.RejectActivity(c, studendId, int64(r.TargetID))
}
