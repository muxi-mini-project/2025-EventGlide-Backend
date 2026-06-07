package service

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
)

var _ InteractionServiceHdl = &InteractionService{}

type InteractionServiceHdl interface {
	Like(context.Context, *req.InteractionReq, string) error
	Dislike(c *gin.Context, r *req.InteractionReq, sid string) error
	Comment(c *gin.Context, r *req.InteractionReq, sid string) error
	Collect(c *gin.Context, r *req.InteractionReq, sid string) error
	DisCollect(c *gin.Context, r *req.InteractionReq, sid string) error
	Approve(c *gin.Context, r *req.InteractionReq, studendId string) error
	Reject(c *gin.Context, r *req.InteractionReq, studendId string) error
}

type InteractionService struct {
	sg SubjectGetter
	id *repo.InteractionRepo
	mq mq.MQHdl
	l  *zap.Logger
}

func NewInteractionService(id *repo.InteractionRepo, mq mq.MQHdl, sg SubjectGetter, l *logger.LoggerSet) *InteractionService {
	return &InteractionService{
		id: id,
		sg: sg,
		mq: mq,
		l:  l.Interaction.Named("service"),
	}
}

func (is *InteractionService) Like(c context.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.sg.GetSubjectInfo(c, r.TargetID, r.Subject)
	if err != nil {
		is.l.Error("Failed to get subject info", zap.Error(err), zap.Int64("targetId", r.TargetID), zap.String("subject", r.Subject))
		return errs.ErrInternal.Wrap(err)
	}
	if sid != ap.StudentID {
		jreq := converter.FeedFromInteractionReq(r, "like", sid, ap.StudentID)
		err = is.mq.Publish(c, "feed_stream", jreq)
		if err != nil {
			is.l.Error("Publish Like Feed Failed", zap.Error(err), zap.Any("feed", jreq))
		} else {
			is.l.Info("Publish Like Feed Success", zap.Any("feed", jreq))
		}
	}

	switch r.Subject {
	case SubjectActivity:
		return is.id.LikeActivity(c, sid, r.TargetID)
	case SubjectPost:
		return is.id.LikePost(c, sid, r.TargetID)
	case SubjectComment:
		return is.id.LikeComment(c, sid, r.TargetID)
	default:
		return errs.ErrInteractionSubjectInvalid
	}
}

func (is *InteractionService) Dislike(c *gin.Context, r *req.InteractionReq, sid string) error {
	switch r.Subject {
	case SubjectActivity:
		return is.id.DislikeActivity(c, sid, r.TargetID)
	case SubjectPost:
		return is.id.DislikePost(c, sid, r.TargetID)
	case SubjectComment:
		return is.id.DislikeComment(c, sid, r.TargetID)
	default:
		return errs.ErrInteractionSubjectInvalid
	}
}

func (is *InteractionService) Comment(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.sg.GetSubjectInfo(c, r.TargetID, r.Subject)
	if err != nil {
		is.l.Error("Failed to get subject info", zap.Error(err), zap.Int64("targetId", r.TargetID), zap.String("subject", r.Subject))
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
		return is.id.CommentActivity(c, sid, r.TargetID)
	case SubjectPost:
		return is.id.CommentPost(c, sid, r.TargetID)
	case SubjectComment:
		return is.id.CommentComment(c, sid, r.TargetID)
	default:
		return errs.ErrInteractionSubjectInvalid
	}
}

func (is *InteractionService) Collect(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.sg.GetSubjectInfo(c, r.TargetID, r.Subject)
	if err != nil {
		is.l.Error("Failed to get subject info", zap.Error(err), zap.Int64("targetId", r.TargetID), zap.String("subject", r.Subject))
		return errs.ErrInternal.Wrap(err)
	}
	if sid != ap.StudentID {
		jreq := converter.FeedFromInteractionReq(r, "collect", sid, ap.StudentID)
		err = is.mq.Publish(c.Request.Context(), "feed_stream", jreq)
		if err != nil {
			is.l.Error("Publish Collect Feed Failed", zap.Error(err), zap.Any("feed", jreq))
		} else {
			is.l.Info("Publish Collect Feed Success", zap.Any("feed", jreq))
		}
	}

	switch r.Subject {
	case SubjectActivity:
		return is.id.CollectActivity(c, sid, r.TargetID)
	case SubjectPost:
		return is.id.CollectPost(c, sid, r.TargetID)
	default:
		return errs.ErrInteractionSubjectInvalid
	}
}

func (is *InteractionService) DisCollect(c *gin.Context, r *req.InteractionReq, sid string) error {
	switch r.Subject {
	case SubjectActivity:
		return is.id.DiscollectActivity(c, sid, r.TargetID)
	case SubjectPost:
		return is.id.DiscollectPost(c, sid, r.TargetID)
	default:
		return errs.ErrInteractionSubjectInvalid
	}
}

func (is *InteractionService) Approve(c *gin.Context, r *req.InteractionReq, studendId string) error {
	return is.id.ApproveActivity(c, studendId, r.TargetID)
}

func (is *InteractionService) Reject(c *gin.Context, r *req.InteractionReq, studendId string) error {
	return is.id.RejectActivity(c, studendId, r.TargetID)
}
