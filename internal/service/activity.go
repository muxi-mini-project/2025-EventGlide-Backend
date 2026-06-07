package service

import (
	"context"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ActivityServiceHdl interface {
	CreateActivity(c context.Context, act *model.Activity, signers []model.Signer, studentID string, aw *req.AuditWrapper) error
	CreateDraft(c context.Context, draft *model.ActivityDraft) error
	LoadDraft(c context.Context, sid string) (model.ActivityDraft, error)
	FindActBySearches(c context.Context, search *req.ActSearchReq) (*model.PaginatedActivities, error)
	FindActByDate(c context.Context, date string, page, limit int) (*model.PaginatedActivities, error)
	FindActByName(c context.Context, name string, page, limit int) (*model.PaginatedActivities, error)
	FindActById(c context.Context, id int64) (model.Activity, error)
	FindActByOwnerID(c context.Context, studentID string, page, limit int) (*model.PaginatedActivities, error)
	ListAllActs(c context.Context, page, limit int) (*model.PaginatedActivities, error)
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

func NewActivityService(ad *repo.ActivityRepo, ud *repo.UserRepo, id *repo.InteractionRepo, mq mq.MQHdl, aud AuditorService, l *logger.LoggerSet) *ActivityService {
	return &ActivityService{
		ad:  ad,
		ud:  ud,
		id:  id,
		aud: aud,
		mq:  mq,
		l:   l.Activity.Named("service"),
	}
}

func (as *ActivityService) CreateActivity(c context.Context, act *model.Activity, signers []model.Signer, studentID string, aw *req.AuditWrapper) error {
	err := as.ad.Transaction(c, func(tx *gorm.DB) error {
		return as.ad.CreateActivity(c, tx, act, signers, studentID)
	})
	if err != nil {
		as.l.Error("failed to create activity tx", zap.Error(err))
		return errs.ErrActivityCreateFailed.Wrap(err)
	}

	go as.publishFeeds(act, signers, studentID)

	as.l.Info("create activity tx",
		zap.Int64("actId", act.Id),
		zap.String("studentID", studentID),
		zap.String("host", act.HolderType),
		zap.String("formfile", act.ActiveForm),
	)

	return nil
}

func (as *ActivityService) CreateDraft(c context.Context, draft *model.ActivityDraft) error {
	if err := as.ad.CreateDraft(c, draft); err != nil {
		as.l.Error("Failed to create draft", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}
	return nil
}

func (as *ActivityService) LoadDraft(c context.Context, sid string) (model.ActivityDraft, error) {
	d, err := as.ad.LoadDraft(c, sid)
	if err != nil {
		as.l.Error("Failed to load draft", zap.Error(err), zap.String("sid", sid))
		return model.ActivityDraft{}, errs.ErrDraftNotFound.Wrap(err)
	}
	return d, nil
}

func (as *ActivityService) FindActBySearches(c context.Context, search *req.ActSearchReq) (*model.PaginatedActivities, error) {
	acts, err := as.ad.FindActBySearches(c, search)
	if err != nil {
		as.l.Error("Failed to search acts", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return acts, nil
}

func (as *ActivityService) FindActByDate(c context.Context, date string, page, limit int) (*model.PaginatedActivities, error) {
	acts, err := as.ad.FindActByDate(c, date, page, limit)
	if err != nil {
		as.l.Error("Failed to find acts by date", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return acts, nil
}

func (as *ActivityService) FindActByName(c context.Context, name string, page, limit int) (*model.PaginatedActivities, error) {
	acts, err := as.ad.FindActByName(c, name, page, limit)
	if err != nil {
		as.l.Error("Failed to find acts by name", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return acts, nil
}

func (as *ActivityService) FindActById(c context.Context, id int64) (model.Activity, error) {
	act, err := as.ad.FindActById(c, id)
	if err != nil {
		as.l.Error("Failed to find act by id", zap.Error(err), zap.Int64("id", id))
		return model.Activity{}, errs.ErrActivityNotFound.Wrap(err)
	}
	return act, nil
}

func (as *ActivityService) FindActByOwnerID(c context.Context, studentID string, page, limit int) (*model.PaginatedActivities, error) {
	acts, err := as.ad.FindActByOwnerID(c, studentID, page, limit)
	if err != nil {
		as.l.Error("Failed to find acts by owner id", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return acts, nil
}

func (as *ActivityService) ListAllActs(c context.Context, page, limit int) (*model.PaginatedActivities, error) {
	acts, err := as.ad.ListAllActs(c, page, limit)
	if err != nil {
		as.l.Error("Failed to list all acts", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return acts, nil
}

func (as *ActivityService) EnrichForSearcher(c context.Context, acts []model.Activity, viewerID string) []model.ActivityDetail {
	studentIDs := make([]string, 0, len(acts)+1)
	studentIDs = append(studentIDs, viewerID)
	for _, act := range acts {
		studentIDs = append(studentIDs, act.StudentID)
	}
	usersMap, _ := as.ud.GetUsersByIDs(c, studentIDs)
	searcher := usersMap[viewerID]

	viewerUserId := int64(0)
	if searcher != nil {
		viewerUserId = int64(searcher.Id)
	}

	details := make([]model.ActivityDetail, 0, len(acts))
	for i := range acts {
		act := &acts[i]
		author := usersMap[act.StudentID]
		if author == nil {
			author = &model.User{}
		}
		details = append(details, model.ActivityDetail{
			Activity: *act,
			Author: model.UserBrief{
				StudentID: author.StudentID,
				Name:      author.Name,
				Avatar:    author.Avatar,
				School:    author.School,
			},
			Images:    act.Images,
			Signers:   act.Signers,
			IsLike:    as.id.IsUserLikedActivity(c, viewerUserId, act.Id),
			IsCollect: as.id.IsUserCollectedActivity(c, viewerUserId, act.Id),
		})
	}
	return details
}

func (as *ActivityService) EnrichOneForSearcher(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail {
	return as.enrichOne(c, act, viewerID)
}

func (as *ActivityService) AuthorBrief(c context.Context, studentID string) model.UserBrief {
	usersMap, _ := as.ud.GetUsersByIDs(c, []string{studentID})
	if len(usersMap) == 0 {
		return model.UserBrief{}
	}
	user := usersMap[studentID]
	if user == nil {
		return model.UserBrief{}
	}
	return model.UserBrief{
		StudentID: user.StudentID,
		Name:      user.Name,
		Avatar:    user.Avatar,
		School:    user.School,
	}
}

func (as *ActivityService) enrichOne(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail {
	usersMap, _ := as.ud.GetUsersByIDs(c, []string{viewerID, act.StudentID})
	searcher := usersMap[viewerID]
	author := usersMap[act.StudentID]
	if searcher == nil {
		searcher = &model.User{}
	}
	if author == nil {
		author = &model.User{}
	}

	viewerUserId := int64(searcher.Id)

	return model.ActivityDetail{
		Activity: *act,
		Author: model.UserBrief{
			StudentID: author.StudentID,
			Name:      author.Name,
			Avatar:    author.Avatar,
			School:    author.School,
		},
		Images:    act.Images,
		Signers:   act.Signers,
		IsLike:    as.id.IsUserLikedActivity(c, viewerUserId, act.Id),
		IsCollect: as.id.IsUserCollectedActivity(c, viewerUserId, act.Id),
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
			TargetId:  act.Id,
			Object:    "activity",
			Action:    "invitation",
			Receiver:  s.StudentID,
		}

		if err := as.mq.Publish(ctx, "feed_stream", f); err != nil {
			as.l.Error("Failed to publish feed", zap.Error(err), zap.String("receiver", s.StudentID), zap.Int64("actId", act.Id))
		}
	}
}

func (as *ActivityService) uploadAuditorForm(ctx context.Context, act *model.Activity, aw *req.AuditWrapper) error {
	form, err := as.aud.CreateAuditorForm(ctx, act.Id, act.ActiveForm, SubjectActivity)
	if err != nil {
		as.l.Error("Failed to create auditor form", zap.Error(err), zap.Int64("actId", act.Id))
		return errs.ErrInternal.Wrap(err)
	}

	err = as.aud.UploadForm(ctx, aw, form.Id)
	if err != nil {
		as.l.Error("Failed to upload form", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
		return errs.ErrInternal.Wrap(err)
	}

	return nil
}

func (as *ActivityService) TriggerAuditorUpload(ctx context.Context, actId int64) error {
	act, err := as.ad.FindActById(ctx, actId)
	if err != nil {
		as.l.Error("Failed to find activity for auditor upload", zap.Error(err), zap.Int64("actId", actId))
		return errs.ErrActivityNotFound.Wrap(err)
	}

	aw := &req.AuditWrapper{
		Subject:   SubjectActivity,
		StudentId: act.StudentID,
	}

	return as.uploadAuditorForm(ctx, &act, aw)
}