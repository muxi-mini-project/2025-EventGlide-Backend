package service

import (
	"context"
	"errors"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/converter"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/safe"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// normalizeActScope 归一化活动列表的时间范围：空值与非法值（含大小写不符）按 active 处理，
// 兼容不传新参数的老客户端。
func normalizeActScope(scope string) string {
	switch scope {
	case req.ActScopeEnded, req.ActScopeAll:
		return scope
	default:
		return req.ActScopeActive
	}
}

// normalizeActRole 归一化活动列表的归属范围：空值与非法值按 creator 处理。
func normalizeActRole(role string) string {
	switch role {
	case req.ActRoleParticipant, req.ActRoleAll:
		return role
	default:
		return req.ActRoleCreator
	}
}

type ActivityServiceHdl interface {
	CreateActivity(c context.Context, act *model.Activity, signers []model.Signer, studentID string, aw *req.AuditWrapper) error
	ResubmitActivity(c context.Context, srcID int64, act *model.Activity, signers []model.Signer, studentID string) error
	IsApprover(c context.Context, act model.Activity, viewer string) bool
	CreateDraft(c context.Context, draft *model.ActivityDraft) error
	LoadDraft(c context.Context, sid string) (model.ActivityDraft, error)
	FindActBySearches(c context.Context, search *req.ActSearchReq) (*model.PaginatedActivities, error)
	FindActByDate(c context.Context, date string, page, limit int) (*model.PaginatedActivities, error)
	FindActByName(c context.Context, name string, page, limit int) (*model.PaginatedActivities, error)
	FindActById(c context.Context, id int64) (model.Activity, error)
	FindActByOwnerID(c context.Context, studentID string, page, limit int) (*model.PaginatedActivities, error)
	FindActListForStudent(c context.Context, studentID, scope, role string, page, limit int) (*model.PaginatedActivities, error)
	ListAllActs(c context.Context, page, limit int) (*model.PaginatedActivities, error)
	EnrichForSearcher(c context.Context, acts []model.Activity, viewerID string) []model.ActivityDetail
	EnrichOneForSearcher(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail
	AuthorBrief(c context.Context, studentID string) model.UserBrief
}

type ActivityService struct {
	ad          *repo.ActivityRepo
	ud          *repo.UserRepo
	id          *repo.InteractionRepo
	mq          mq.MQHdl
	aud         AuditorService
	imgWorker   imageWorkerNotifier
	maxResubmit int
	effect      string
	l           *zap.Logger
}

func NewActivityService(ad *repo.ActivityRepo, ud *repo.UserRepo, id *repo.InteractionRepo, mq mq.MQHdl, aud AuditorService, imgWorker imageWorkerNotifier, l *logger.LoggerSet, cfg *config.Conf) *ActivityService {
	lg := l.Activity.Named("service")
	effect := cfg.Auditor.EffectMode()
	if effect != cfg.Auditor.Effect {
		lg.Warn("auditor.effect 未配置或非法，活动可见性按 slow 处理",
			zap.String("effect", cfg.Auditor.Effect))
	}
	return &ActivityService{
		ad:          ad,
		ud:          ud,
		id:          id,
		aud:         aud,
		mq:          mq,
		imgWorker:   imgWorker,
		maxResubmit: cfg.Auditor.ResubmitLimit(),
		effect:      effect,
		l:           lg,
	}
}

// CanViewActivity 判断 viewer 能否查看该活动，规则见 ActivityVisible。
func (as *ActivityService) CanViewActivity(act model.Activity, viewer string, isSigner bool) bool {
	return ActivityVisible(act.IsChecking, act.StudentID, viewer, isSigner, as.effect)
}

func (as *ActivityService) CreateActivity(c context.Context, act *model.Activity, signers []model.Signer, studentID string, aw *req.AuditWrapper) error {
	// 前端为准：宽高随请求入库；漏传/传错的行走 pending，创建不阻塞，
	// 落库成功后唤醒回填 worker 异步兜底。
	// pending 必须在事务前判定：DAO 写入活动时会把 act.Images 置空，事务后读不到图片行。
	pending := hasPendingImage(act.Images)

	err := as.ad.Transaction(c, func(tx *gorm.DB) error {
		return as.ad.CreateActivity(c, tx, act, signers, studentID)
	})
	if err != nil {
		as.l.Error("failed to create activity tx", zap.Error(err))
		return errs.ErrActivityCreateFailed.Wrap(err)
	}

	if pending {
		as.imgWorker.Notify()
	}

	safe.Go(as.l, "publishFeeds", func() { as.publishFeeds(act, signers, studentID) })

	as.l.Info("create activity tx",
		zap.Int64("actId", act.Id),
		zap.String("studentID", studentID),
		zap.String("host", act.HolderType),
		zap.String("formfile", act.ActiveForm),
	)

	return nil
}

// errResubmitConflict 表示源活动的条件更新未命中：已被替代，或在读取之后被平台改判。
var errResubmitConflict = errors.New("source activity no longer resubmittable")

// ResubmitActivity 重新送审被驳回的活动：用用户编辑后的完整内容创建一条新申请（新 revision、
// 新的签署人审批记录），并在同一事务内把源活动的 replaced_by 指向新活动。源活动与其审核表单保留。
// 新活动的审核表单由后台送审 worker 在下一轮创建并推送，此处不复制。
func (as *ActivityService) ResubmitActivity(c context.Context, srcID int64, act *model.Activity, signers []model.Signer, studentID string) error {
	src, err := as.ad.FindActById(c, srcID)
	if err != nil {
		as.l.Error("Failed to find source activity for resubmit", zap.Error(err), zap.Int64("srcId", srcID))
		return errs.ErrActivityNotFound.Wrap(err)
	}
	if err := validateResubmit(src, studentID, as.maxResubmit); err != nil {
		return err
	}

	act.Revision = src.Revision + 1

	// pending 必须在事务前判定：DAO 写入活动时会把 act.Images 置空，事务后读不到图片行。
	pending := hasPendingImage(act.Images)

	conflict := false
	err = as.ad.Transaction(c, func(tx *gorm.DB) error {
		// 先条件更新源活动的 replaced_by：并发重复提交时只有一个事务能把 0 改为新 id，
		// 其余请求在这里即被判为已替代，不依赖唯一索引兜底，错误码更准确。
		// 条件同时含 is_checking = reject，避免把刚被平台改判为通过的活动替换掉。
		replaced, err := as.ad.MarkReplaced(c, tx, srcID, act.Id)
		if err != nil {
			return err
		}
		if !replaced {
			conflict = true
			return errResubmitConflict
		}
		// 保留用户正在编辑的其它活动草稿：重送不是新建活动。
		return as.ad.CreateActivityKeepDraft(c, tx, act, signers, studentID)
	})
	if err != nil {
		if conflict {
			return as.classifyResubmitConflict(c, srcID)
		}
		as.l.Error("failed to resubmit activity tx", zap.Error(err), zap.Int64("srcId", srcID))
		return errs.ErrActivityCreateFailed.Wrap(err)
	}

	if pending {
		as.imgWorker.Notify()
	}

	as.l.Info("resubmit activity tx",
		zap.Int64("srcId", srcID),
		zap.Int64("actId", act.Id),
		zap.Int("revision", act.Revision),
	)

	// 与新建一致：新申请需要重新通知签署人签署（源申请的签署记录不能复用）。
	safe.Go(as.l, "publishFeeds", func() { as.publishFeeds(act, signers, studentID) })

	return nil
}

// classifyResubmitConflict 在条件更新未命中后重新读取源活动，返回准确的错误码：
// 已被新申请替代与已被平台改判是两种不同的用户可见结果，不能都报"已替代"。
func (as *ActivityService) classifyResubmitConflict(c context.Context, srcID int64) error {
	src, err := as.ad.FindActById(c, srcID)
	if err != nil {
		as.l.Warn("failed to reload source activity after resubmit conflict",
			zap.Error(err), zap.Int64("srcId", srcID))
		return errs.ErrActivityAlreadyReplaced
	}
	if src.ReplacedBy != 0 {
		return errs.ErrActivityAlreadyReplaced
	}
	if src.IsChecking != model.StanceReject {
		return errs.ErrActivityNotRejected
	}
	return errs.ErrActivityAlreadyReplaced
}

// validateResubmit 校验重新送审的前置条件：仅创建者可操作，源活动必须被驳回且未被替代，
// 且新的 revision 不超过配置的次数上限。
func validateResubmit(src model.Activity, studentID string, maxResubmit int) error {
	if src.StudentID != studentID {
		return errs.ErrActivityNotOwner
	}
	if src.IsChecking != model.StanceReject {
		return errs.ErrActivityNotRejected
	}
	if src.ReplacedBy != 0 {
		return errs.ErrActivityAlreadyReplaced
	}
	if src.Revision+1 > maxResubmit {
		return errs.ErrActivityResubmitLimit
	}
	return nil
}

// IsApprover 判断 viewer 是否是该活动的签署人（approvement 中登记过）。
// 创建者不算签署人，权限判断由调用方另行处理。
func (as *ActivityService) IsApprover(c context.Context, act model.Activity, viewer string) bool {
	if viewer == "" || act.StudentID == viewer {
		return false
	}
	ok, err := as.ad.ExistsApprovement(c, act.Id, viewer)
	if err != nil {
		as.l.Error("Failed to check activity approver", zap.Error(err), zap.Int64("actId", act.Id))
		return false
	}
	return ok
}

func (as *ActivityService) CreateDraft(c context.Context, draft *model.ActivityDraft) error {
	if err := as.ad.CreateDraft(c, draft); err != nil {
		as.l.Error("Failed to create draft", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}
	// 草稿图片同样可能漏传宽高：唤醒回填 worker 兜底。
	// 草稿反复保存会整批删旧插新，同 URL 行由 CopySizeFromDuplicates 免费复制。
	if hasPendingImage(draft.Images) {
		as.imgWorker.Notify()
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

// FindActListForStudent 返回学生本人视角的活动列表：默认只含自己创建的活动，
// role 可扩展到"我签署的"；scope 控制时间范围与已结束活动的审核状态。
// 不套 SetEffect：本人要能看见自己的审核中与被拒活动。
func (as *ActivityService) FindActListForStudent(c context.Context, studentID string, scope string, role string, page, limit int) (*model.PaginatedActivities, error) {
	acts, err := as.ad.FindActListForStudent(c, studentID, normalizeActScope(scope), normalizeActRole(role), page, limit)
	if err != nil {
		as.l.Error("Failed to find act list for student", zap.Error(err), zap.String("studentID", studentID))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return acts, nil
}

// RejectReasons 返回给定活动中审核不通过者的拒绝原因（activity id -> reason）。
// 仅 is_checking='reject' 的活动返回原因：平台在 pass 时也可能带 msg，不能当作拒绝原因展示。
// 查询失败只记日志返回空，不阻断列表/详情。
func (as *ActivityService) RejectReasons(c context.Context, acts ...model.Activity) map[int64]string {
	ids := make([]int64, 0, len(acts))
	for _, act := range acts {
		if act.IsChecking == model.StanceReject {
			ids = append(ids, act.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	reasons, err := as.aud.GetReasons(c, ids)
	if err != nil {
		as.l.Error("Failed to get auditor reject reasons", zap.Error(err))
		return nil
	}
	return reasons
}

func (as *ActivityService) EnrichForSearcher(c context.Context, acts []model.Activity, viewerID string) []model.ActivityDetail {
	studentIDs := make([]string, 0, len(acts)+1)
	studentIDs = append(studentIDs, viewerID)
	for _, act := range acts {
		studentIDs = append(studentIDs, act.StudentID)
	}
	usersMap, err := as.ud.GetUsersByIDs(c, studentIDs)
	if err != nil {
		as.l.Error("Failed to batch get users when enriching activities", zap.Error(err))
	}
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

func (as *ActivityService) EnrichForSearcherWithStatuses(c context.Context, acts []model.Activity, viewerID string, likedMap, collectedMap map[int64]bool) []model.ActivityDetail {
	studentIDs := make([]string, 0, len(acts)+1)
	studentIDs = append(studentIDs, viewerID)
	for _, act := range acts {
		studentIDs = append(studentIDs, act.StudentID)
	}
	usersMap, err := as.ud.GetUsersByIDs(c, studentIDs)
	if err != nil {
		as.l.Error("Failed to batch get users when enriching activities", zap.Error(err))
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
			IsLike:    likedMap[act.Id],
			IsCollect: collectedMap[act.Id],
		})
	}
	return details
}

func (as *ActivityService) EnrichOneForSearcher(c context.Context, act *model.Activity, viewerID string) model.ActivityDetail {
	return as.enrichOne(c, act, viewerID)
}

func (as *ActivityService) AuthorBrief(c context.Context, studentID string) model.UserBrief {
	usersMap, err := as.ud.GetUsersByIDs(c, []string{studentID})
	if err != nil {
		as.l.Error("Failed to get user when building author brief", zap.Error(err), zap.String("studentID", studentID))
	}
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
	usersMap, err := as.ud.GetUsersByIDs(c, []string{viewerID, act.StudentID})
	if err != nil {
		as.l.Error("Failed to batch get users when enriching activity", zap.Error(err))
	}
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
	if err := as.aud.MarkPushed(ctx, form.Id, time.Now()); err != nil {
		as.l.Error("Failed to mark form pushed", zap.Error(err), zap.Int64("actId", act.Id), zap.Int64("formId", form.Id))
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
		CactReq:   converter.ActivityToAuditReq(&act),
	}

	return as.uploadAuditorForm(ctx, &act, aw)
}
