package service

import (
	"context"
	"strings"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"go.uber.org/zap"
)

type ActivityServiceHdl interface {
	CreateActivity(c context.Context, act *model.Activity, signers []model.Signer, studentID string, aw *req.AuditWrapper) error
	CreateDraft(c context.Context, draft *model.ActivityDraft) error
	LoadDraft(c context.Context, sid string) (model.ActivityDraft, error)
	FindActBySearches(c context.Context, search *req.ActSearchReq) ([]model.Activity, error)
	FindActByDate(c context.Context, date string) ([]model.Activity, error)
	FindActByName(c context.Context, name string) ([]model.Activity, error)
	FindActByBid(c context.Context, bid string) (model.Activity, error)
	FindActByOwnerID(c context.Context, studentID string) ([]model.Activity, error)
	ListAllActs(c context.Context) ([]model.Activity, error)
	EnrichForSearcher(c context.Context, acts []model.Activity, viewerID string) []model.ActivityDetail
	EnrichOneForSearcher(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail
	AuthorBrief(c context.Context, studentID string) model.UserBrief
}

type ActivityService struct {
	ad  *repo.ActivityRepo
	ud  *repo.UserRepo
	id  *repo.InteractionRepo
	mq  mq.MQHdl
	aud AuditorService
	l   *zap.Logger
}

func NewActivityService(ad *repo.ActivityRepo, ud *repo.UserRepo, l *zap.Logger, id *repo.InteractionRepo, mq mq.MQHdl, aud AuditorService) *ActivityService {
	return &ActivityService{
		ad:  ad,
		ud:  ud,
		id:  id,
		aud: aud,
		mq:  mq,
		l:   l.Named("activity/service"),
	}
}

func (as *ActivityService) CreateActivity(c context.Context, act *model.Activity, signers []model.Signer, studentID string, aw *req.AuditWrapper) error {
	err := as.ad.CreateActivityTx(c, act, signers, studentID)
	if err != nil {
		as.l.Error("failed to create activity tx", zap.Error(err))
		return err
	}

	go as.retryUploadAuditorForm(act, aw)

	go as.publishFeeds(act, signers, studentID)

	as.l.Info("create activity tx",
		zap.String("act", act.Bid),
		zap.String("studentID", studentID),
		zap.String("host", act.HolderType),
		zap.String("formfile", act.ActiveForm),
		zap.String("signer", act.Signer),
	)

	return nil
}

func (as *ActivityService) CreateDraft(c context.Context, draft *model.ActivityDraft) error {
	err := as.ad.CreateDraft(c, draft)
	if err != nil {
		as.l.Error("Failed to create draft", zap.Error(err))
		return err
	}
	as.l.Info("create draft", zap.String("draft", draft.Bid), zap.String("student", draft.StudentID))
	return nil
}

func (as *ActivityService) LoadDraft(c context.Context, sid string) (model.ActivityDraft, error) {
	d, err := as.ad.LoadDraft(c, sid)
	if err != nil {
		return model.ActivityDraft{}, err
	}
	return d, nil
}

func (as *ActivityService) FindActBySearches(c context.Context, search *req.ActSearchReq) ([]model.Activity, error) {
	acts, err := as.ad.FindActBySearches(c, search)
	if err != nil {
		as.l.Error("Failed to search acts", zap.Error(err))
		return nil, err
	}
	return acts, nil
}

func (as *ActivityService) FindActByDate(c context.Context, date string) ([]model.Activity, error) {
	return as.ad.FindActByDate(c, date)
}

func (as *ActivityService) FindActByName(c context.Context, name string) ([]model.Activity, error) {
	return as.ad.FindActByName(c, name)
}

func (as *ActivityService) FindActByBid(c context.Context, bid string) (model.Activity, error) {
	return as.ad.FindActByBid(c, bid)
}

func (as *ActivityService) FindActByOwnerID(c context.Context, studentID string) ([]model.Activity, error) {
	return as.ad.FindActByOwnerID(c, studentID)
}

func (as *ActivityService) ListAllActs(c context.Context) ([]model.Activity, error) {
	return as.ad.ListAllActs(c)
}

func (as *ActivityService) EnrichForSearcher(c context.Context, acts []model.Activity, viewerID string) []model.ActivityDetail {
	details := make([]model.ActivityDetail, 0, len(acts))
	for i := range acts {
		details = append(details, as.enrichOne(c, &acts[i], viewerID))
	}
	return details
}

func (as *ActivityService) EnrichOneForSearcher(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail {
	return as.enrichOne(c, act, viewerID)
}

func (as *ActivityService) AuthorBrief(c context.Context, studentID string) model.UserBrief {
	user := as.ud.FindUserByID(c, studentID)
	return model.UserBrief{
		StudentID: user.StudentID,
		Name:      user.Name,
		Avatar:    user.Avatar,
		School:    user.School,
	}
}

func (as *ActivityService) enrichOne(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail {
	searcher := as.ud.FindUserByID(c, viewerID)
	author := as.ud.FindUserByID(c, act.StudentID)

	return model.ActivityDetail{
		Activity: *act,
		Author: model.UserBrief{
			StudentID: author.StudentID,
			Name:      author.Name,
			Avatar:    author.Avatar,
			School:    author.School,
		},
		IsLike:    strings.Contains(searcher.LikeAct, act.Bid),
		IsCollect: strings.Contains(searcher.CollectAct, act.Bid),
	}
}

func (as *ActivityService) publishFeeds(act *model.Activity, signers []model.Signer, studentID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, s := range signers {
		if s.StudentID == studentID {
			continue
		}

		f := model.Feed{
			StudentID: studentID,
			TargetBid: act.Bid,
			Object:    "activity",
			Action:    "invitation",
			Receiver:  s.StudentID,
		}

		if err := as.mq.Publish(ctx, "feed_stream", f); err != nil {
			as.l.Error("Failed to publish feed", zap.Error(err), zap.String("receiver", s.StudentID), zap.String("actBid", act.Bid))
		}
	}
}

func (as *ActivityService) retryUploadAuditorForm(act *model.Activity, aw *req.AuditWrapper) {
	const maxRetry = 5
	for i := 1; i <= maxRetry; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := as.uploadAuditorForm(ctx, act, aw)
		cancel()
		if err == nil {
			as.l.Info("Upload auditor form success", zap.String("actBid", act.Bid), zap.Int("retry", i))
			return
		}

		as.l.Error("Upload auditor form failed", zap.Error(err), zap.String("actBid", act.Bid), zap.Int("retry", i))
		time.Sleep(time.Duration(i*i) * time.Second)
	}

	as.l.Error("Upload auditor form finally failed", zap.String("actBid", act.Bid))
}

func (as *ActivityService) uploadAuditorForm(ctx context.Context, act *model.Activity, aw *req.AuditWrapper) error {
	form, err := as.aud.CreateAuditorForm(ctx, act.Bid, act.ActiveForm, SubjectActivity)
	if err != nil {
		return err
	}

	err = as.aud.UploadForm(ctx, aw, form.Id)
	if err != nil {
		return err
	}

	return nil
}
