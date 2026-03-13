package service

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"go.uber.org/zap"
)

type InteractionService struct {
	apg ActPostCommentGetter
	id  *dao.InteractionDao
	mq  mq.MQHdl
	l   *zap.Logger
}

func NewInteractionService(id *dao.InteractionDao, mq mq.MQHdl, l *zap.Logger, apg ActPostCommentGetter) *InteractionService {
	return &InteractionService{
		id:  id,
		apg: apg,
		mq:  mq,
		l:   l.Named("interaction/service"),
	}
}

func (is *InteractionService) Like(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.apg.GetActivityOrPostOrComment(c, r.TargetID, r.Subject)
	if err != nil {
		return err
	}
	jreq := is.toFeed(r, "like", sid, ap.GetStudentID())
	err = is.mq.Publish(c.Request.Context(), "feed_stream", jreq)
	if err != nil {
		is.l.Error("Publish Like Feed Failed", zap.Error(err), zap.Any("feed", jreq))
	} else {
		is.l.Info("Publish Like Feed Success", zap.Any("feed", jreq))
	}

	switch r.Subject {
	case "activity":
		return is.id.LikeActivity(c, sid, r.TargetID)
	case "post":
		return is.id.LikePost(c, sid, r.TargetID)
	case "comment":
		return is.id.LikeComment(c, sid, r.TargetID)
	default:
		return errors.New("subject error")
	}
}

func (is *InteractionService) Dislike(c *gin.Context, r *req.InteractionReq, sid string) error {
	switch r.Subject {
	case "activity":
		return is.id.DislikeActivity(c, sid, r.TargetID)
	case "post":
		return is.id.DislikePost(c, sid, r.TargetID)
	case "comment":
		return is.id.DislikeComment(c, sid, r.TargetID)
	default:
		return errors.New("subject error")
	}
}

func (is *InteractionService) Comment(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.apg.GetActivityOrPostOrComment(c, r.TargetID, r.Subject)
	if err != nil {
		return err
	}
	jreq := is.toFeed(r, "comment", sid, ap.GetStudentID())
	err = is.mq.Publish(c.Request.Context(), "feed_stream", jreq)
	if err != nil {
		is.l.Error("Publish Comment Feed Failed", zap.Error(err), zap.Any("feed", jreq))
	} else {
		is.l.Info("Publish Comment Feed Success", zap.Any("feed", jreq))
	}

	switch r.Subject {
	case "activity":
		return is.id.CommentActivity(c, sid, r.TargetID)
	case "post":
		return is.id.CommentPost(c, sid, r.TargetID)
	case "comment":
		return is.id.CommentComment(c, sid, r.TargetID)
	default:
		return errors.New("subject error")
	}
}

func (is *InteractionService) Collect(c *gin.Context, r *req.InteractionReq, sid string) error {
	ap, err := is.apg.GetActivityOrPostOrComment(c, r.TargetID, r.Subject)
	if err != nil {
		return err
	}
	jreq := is.toFeed(r, "collect", sid, ap.GetStudentID())
	err = is.mq.Publish(c.Request.Context(), "feed_stream", jreq)
	if err != nil {
		is.l.Error("Publish Collect Feed Failed", zap.Error(err), zap.Any("feed", jreq))
	} else {
		is.l.Info("Publish Collect Feed Success", zap.Any("feed", jreq))
	}

	switch r.Subject {
	case "activity":
		return is.id.CollectActivity(c, sid, r.TargetID)
	case "post":
		return is.id.CollectPost(c, sid, r.TargetID)
	default:
		return errors.New("subject error")
	}
}

func (is *InteractionService) Discollect(c *gin.Context, r *req.InteractionReq, sid string) error {
	switch r.Subject {
	case "activity":
		return is.id.DiscollectActivity(c, sid, r.TargetID)
	case "post":
		return is.id.DiscollectPost(c, sid, r.TargetID)
	default:
		return errors.New("subject error")
	}
}

func (is *InteractionService) toFeed(r *req.InteractionReq, action string, sid string, recv string) model.Feed {
	f := model.Feed{
		TargetBid: r.TargetID,
		Object:    r.Subject,
		StudentId: sid,
		Action:    action,
		Receiver:  recv,
	}
	return f
}

func (is *InteractionService) Approve(c *gin.Context, r *req.InteractionReq, studendId string) error {
	return is.id.ApproveActivity(c, studendId, r.TargetID)
}

func (is *InteractionService) Reject(c *gin.Context, r *req.InteractionReq, studendId string) error {
	return is.id.RejectActivity(c, studendId, r.TargetID)
}
