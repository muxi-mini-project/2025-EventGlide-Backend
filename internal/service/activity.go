package service

import (
	"context"
	"time"

	"github.com/raiki02/EG/api/req"
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
		return err
	}

	go as.retryUploadAuditorForm(act, aw)

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
	return as.ad.CreateDraft(c, draft)
}

func (as *ActivityService) LoadDraft(c context.Context, sid string) (model.ActivityDraft, error) {
	d, err := as.ad.LoadDraft(c, sid)
	if err != nil {
		return model.ActivityDraft{}, err
	}
	return d, nil
}

func (as *ActivityService) FindActBySearches(c context.Context, search *req.ActSearchReq) (*model.PaginatedActivities, error) {
	acts, err := as.ad.FindActBySearches(c, search)
	if err != nil {
		as.l.Error("Failed to search acts", zap.Error(err))
		return nil, err
	}
	return acts, nil
}

func (as *ActivityService) FindActByDate(c context.Context, date string, page, limit int) (*model.PaginatedActivities, error) {
	return as.ad.FindActByDate(c, date, page, limit)
}

func (as *ActivityService) FindActByName(c context.Context, name string, page, limit int) (*model.PaginatedActivities, error) {
	return as.ad.FindActByName(c, name, page, limit)
}

func (as *ActivityService) FindActById(c context.Context, id int64) (model.Activity, error) {
	return as.ad.FindActById(c, id)
}

func (as *ActivityService) FindActByOwnerID(c context.Context, studentID string, page, limit int) (*model.PaginatedActivities, error) {
	return as.ad.FindActByOwnerID(c, studentID, page, limit)
}

func (as *ActivityService) ListAllActs(c context.Context, page, limit int) (*model.PaginatedActivities, error) {
	return as.ad.ListAllActs(c, page, limit)
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

func (as *ActivityService) retryUploadAuditorForm(act *model.Activity, aw *req.AuditWrapper) {
	const maxRetry = 5
	for i := 1; i <= maxRetry; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := as.uploadAuditorForm(ctx, act, aw)
		cancel()
		if err == nil {
			as.l.Info("Upload auditor form success", zap.Int64("actId", act.Id), zap.Int("retry", i))
			return
		}

		as.l.Error("Upload auditor form failed", zap.Error(err), zap.Int64("actId", act.Id), zap.Int("retry", i))
		if i < maxRetry {
			time.Sleep(time.Duration(i*i) * time.Second)
		}
	}

	as.l.Error("Upload auditor form finally failed", zap.Int64("actId", act.Id))
}

func (as *ActivityService) uploadAuditorForm(ctx context.Context, act *model.Activity, aw *req.AuditWrapper) error {
	form, err := as.aud.CreateAuditorForm(ctx, act.Id, act.ActiveForm, SubjectActivity)
	if err != nil {
		return err
	}

	err = as.aud.UploadForm(ctx, aw, form.Id)
	if err != nil {
		return err
	}

	return nil
}