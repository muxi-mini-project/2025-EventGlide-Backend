package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/encrypt"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

var _ UserServiceHdl = &UserService{}

type UserServiceHdl interface {
	CreateUser(context.Context, string, string, string) error
	Login(context.Context, string, string) (*model.User, string, error)
	Logout(context.Context, string) error
	GetUserInfo(context.Context, string) (*model.User, error)
	UpdateAvatar(context.Context, req.UserAvatarReq, string) error
	UpdateUsername(context.Context, string, string) error
	SearchUserAct(context.Context, string, string, int, int) ([]model.ActivityDetail, error)
	SearchUserPost(context.Context, string, string, int, int) ([]model.PostDetail, error)
	GenQINIUToken(context.Context) (string, string)
	GetChecking(context.Context, string) ([]model.ActivityDetail, []model.PostDetail, error)
	LoadCollectAct(ctx context.Context, studentId string, page, limit int) (*model.PaginatedActivities, error)
	LoadCollectPost(ctx context.Context, studentId string, page, limit int) (*model.PaginatedPosts, error)
	LoadLikePost(ctx context.Context, studentId string, page, limit int) (*model.PaginatedPosts, error)
	LoadLikeAct(ctx context.Context, studentId string, page, limit int) (*model.PaginatedActivities, error)
	VerifyUser(ctx context.Context, studentId string, realName string) (bool, error)
}

type UserService struct {
	udh  *repo.UserRepo
	adh  *repo.ActivityRepo
	pdh  *repo.PostRepo
	idh  *repo.InteractionRepo
	jwth *middleware.Jwt
	cSvc StudentCrawler
	iuh  *ImgUploader
	as   *ActivityService
	ps   *PostService
	l    *zap.Logger
	cfg  *config.Conf
}

func NewUserService(udh *repo.UserRepo, adh *repo.ActivityRepo, pdh *repo.PostRepo, ih *repo.InteractionRepo, jwth *middleware.Jwt, cSvc StudentCrawler, iuh *ImgUploader, as *ActivityService, ps *PostService, l *logger.LoggerSet, cfg *config.Conf) *UserService {
	return &UserService{
		udh:  udh,
		adh:  adh,
		pdh:  pdh,
		idh:  ih,
		jwth: jwth,
		cSvc: cSvc,
		iuh:  iuh,
		as:   as,
		ps:   ps,
		l:    l.User.Named("service"),
		cfg:  cfg,
	}
}

func (us *UserService) CreateUser(ctx context.Context, sid string, name string, department string) error {
	user := &model.User{
		StudentID: sid,
		Name:      tools.GenRandomUsername(sid),
		RealName:  model.EncryptedString(name),
		Avatar:    us.cfg.Imgbed.DefaultAvatar1,
		School:    "华中师范大学",
		College:   department,
	}
	if err := us.udh.Create(ctx, user); err != nil {
		us.l.Error("Failed to create user", zap.Error(err), zap.String("studentId", sid))
		return errs.ErrInternal.Wrap(err)
	}
	return nil
}

func (us *UserService) Login(ctx context.Context, studentId string, password string) (*model.User, string, error) {
	client, err := us.cSvc.Login(ctx, studentId, password)
	if err != nil {
		us.l.Error("Login failed", zap.Error(err), zap.String("studentId", studentId))
		if errors.Is(err, errs.ErrLoginFailed) {
			return nil, "", errs.ErrLoginFailed
		}
		if errors.Is(err, errs.ErrNetworkError) {
			return nil, "", errs.ErrNetworkError
		}
		if errors.Is(err, errs.ErrLoginInfoInvalid) {
			return nil, "", errs.ErrLoginInfoInvalid
		}
		return nil, "", errs.ErrInternal.Wrap(err)
	}
	if client == nil {
		return nil, "", errs.ErrLoginFailed
	}

	var name, department string
	supportsUserInfo := us.cSvc.SupportsUserInfo(studentId)
	if supportsUserInfo {
		name, department, err = us.cSvc.GetNameAndDepartment(ctx, studentId, client)
		if err != nil {
			us.l.Warn("get user info failed", zap.Error(err))
			name = ""
			department = ""
		}
	}

	if !us.udh.CheckUserExist(ctx, studentId) {
		err = us.CreateUser(ctx, studentId, name, department)
		if err != nil {
			us.l.Error("Create user failed", zap.Error(err), zap.String("studentId", studentId))
			return nil, "", errs.ErrInternal.Wrap(err)
		}
	}

	token := us.jwth.GenToken(ctx, studentId)
	err = us.jwth.StoreInRedis(ctx, studentId, token)
	if err != nil {
		us.l.Error("Store token in redis failed", zap.Error(err), zap.String("studentId", studentId))
		return nil, "", errs.ErrInternal.Wrap(err)
	}

	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		us.l.Error("Get user info failed", zap.Error(err), zap.String("studentId", studentId))
		if errors.Is(err, encrypt.ErrDecrypt) {
			return nil, "", errs.ErrInternal.Wrap(err)
		}
		return nil, "", errs.ErrUserNotFound.Wrap(err)
	}

	if supportsUserInfo && (user.RealName == "" || user.College == "") {
		go us.loadUserInfoAsync(client, studentId)
	}

	return &user, token, nil
}

func (us *UserService) Logout(ctx context.Context, token string) error {
	err := us.jwth.ClearToken(ctx, token)
	if err != nil {
		us.l.Error("Clear token failed", zap.Error(err))
		return errs.ErrInternal.Wrap(err)
	}
	return nil
}

func (us *UserService) GetUserInfo(ctx context.Context, studentId string) (*model.User, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get user info", zap.Error(err), zap.String("studentId", studentId))
		if errors.Is(err, encrypt.ErrDecrypt) {
			return nil, errs.ErrInternal.Wrap(err)
		}
		return nil, errs.ErrUserNotFound.Wrap(err)
	}
	return &user, nil
}

func (us *UserService) UpdateAvatar(ctx context.Context, req req.UserAvatarReq, sid string) error {
	if err := us.udh.UpdateAvatar(ctx, sid, req.AvatarUrl); err != nil {
		us.l.Error("Failed to update avatar", zap.Error(err), zap.String("studentId", sid))
		return errs.ErrInternal.Wrap(err)
	}
	return nil
}

func (us *UserService) UpdateUsername(ctx context.Context, studentId string, name string) error {
	if err := us.udh.UpdateUsername(ctx, studentId, name); err != nil {
		us.l.Error("Failed to update username", zap.Error(err), zap.String("studentId", studentId))
		return errs.ErrInternal.Wrap(err)
	}
	return nil
}

func (us *UserService) SearchUserAct(ctx context.Context, studentId string, keyword string, page int, limit int) ([]model.ActivityDetail, error) {
	acts, err := us.adh.FindActByUser(ctx, studentId, keyword, page, limit)
	if err != nil {
		us.l.Error("Failed to search user acts", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return us.as.EnrichForSearcher(ctx, acts.Acts, studentId), nil
}

func (us *UserService) SearchUserPost(ctx context.Context, studentId string, keyword string, page, limit int) ([]model.PostDetail, error) {
	posts, err := us.pdh.FindPostByUser(ctx, studentId, keyword, page, limit)
	if err != nil {
		us.l.Error("Failed to search user posts", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrInternal.Wrap(err)
	}
	return us.ps.EnrichForSearcher(ctx, posts.Posts, studentId), nil
}

func (us *UserService) GetChecking(ctx context.Context, studentId string) ([]model.ActivityDetail, []model.PostDetail, error) {
	acts, err := us.adh.GetChecking(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get checking acts", zap.Error(err), zap.String("studentId", studentId))
		return nil, nil, errs.ErrInternal.Wrap(err)
	}
	actDetails := us.as.EnrichForSearcher(ctx, acts, studentId)

	posts, err := us.pdh.GetChecking(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get checking posts", zap.Error(err), zap.String("studentId", studentId))
		return nil, nil, errs.ErrInternal.Wrap(err)
	}
	postDetails := us.ps.EnrichForSearcher(ctx, posts, studentId)

	return actDetails, postDetails, nil
}

func (us *UserService) GenQINIUToken(ctx context.Context) (string, string) {
	return us.iuh.GenQINIUToken(ctx), us.iuh.ImgUrl
}

func (us *UserService) LoadCollectAct(ctx context.Context, studentId string, page, limit int) (*model.PaginatedActivities, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get user info", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrUserNotFound.Wrap(err)
	}

	paginatedIds, err := us.idh.GetUserCollectedActivityIds(ctx, int64(user.Id), page, limit)
	if err != nil {
		us.l.Error("Failed to get collected activity ids", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrInternal.Wrap(err)
	}

	if len(paginatedIds.Ids) == 0 {
		return &model.PaginatedActivities{Total: paginatedIds.Total, Page: page, Limit: limit, Acts: []model.Activity{}}, nil
	}

	acts, err := us.adh.FindActsByIds(ctx, paginatedIds.Ids)
	if err != nil {
		us.l.Error("Failed to find activities by ids", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}

	actMap := make(map[int64]model.Activity)
	for _, act := range acts {
		actMap[act.Id] = act
	}

	var orderedActs []model.Activity
	for _, id := range paginatedIds.Ids {
		if act, ok := actMap[id]; ok {
			orderedActs = append(orderedActs, act)
		}
	}

	likedIds, collectedIds, _ := us.idh.GetUserActivityInteractionStatuses(ctx, int64(user.Id), paginatedIds.Ids)
	likedMap := make(map[int64]bool)
	collectedMap := make(map[int64]bool)
	for _, id := range likedIds {
		likedMap[id] = true
	}
	for _, id := range collectedIds {
		collectedMap[id] = true
	}

	details := us.as.EnrichForSearcherWithStatuses(ctx, orderedActs, studentId, likedMap, collectedMap)
	actsOut := make([]model.Activity, 0, len(details))
	for _, d := range details {
		actsOut = append(actsOut, d.Activity)
	}
	return &model.PaginatedActivities{Total: paginatedIds.Total, Page: page, Limit: limit, Acts: actsOut}, nil
}

func (us *UserService) LoadCollectPost(ctx context.Context, studentId string, page, limit int) (*model.PaginatedPosts, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get user info", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrUserNotFound.Wrap(err)
	}

	paginatedIds, err := us.idh.GetUserCollectedPostIds(ctx, int64(user.Id), page, limit)
	if err != nil {
		us.l.Error("Failed to get collected post ids", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrInternal.Wrap(err)
	}

	if len(paginatedIds.Ids) == 0 {
		return &model.PaginatedPosts{Total: paginatedIds.Total, Page: page, Limit: limit, Posts: []model.Post{}}, nil
	}

	posts, err := us.pdh.FindPostsByIds(ctx, paginatedIds.Ids)
	if err != nil {
		us.l.Error("Failed to find posts by ids", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}

	postMap := make(map[int64]model.Post)
	for _, p := range posts {
		postMap[p.Id] = p
	}

	var orderedPosts []model.Post
	for _, id := range paginatedIds.Ids {
		if post, ok := postMap[id]; ok {
			orderedPosts = append(orderedPosts, post)
		}
	}

	likedIds, collectedIds, _ := us.idh.GetUserPostInteractionStatuses(ctx, int64(user.Id), paginatedIds.Ids)
	likedMap := make(map[int64]bool)
	collectedMap := make(map[int64]bool)
	for _, id := range likedIds {
		likedMap[id] = true
	}
	for _, id := range collectedIds {
		collectedMap[id] = true
	}

	details := us.ps.EnrichForSearcherWithStatuses(ctx, orderedPosts, studentId, likedMap, collectedMap)
	postsOut := make([]model.Post, 0, len(details))
	for _, d := range details {
		postsOut = append(postsOut, d.Post)
	}
	return &model.PaginatedPosts{Total: paginatedIds.Total, Page: page, Limit: limit, Posts: postsOut}, nil
}

func (us *UserService) LoadLikePost(ctx context.Context, studentId string, page, limit int) (*model.PaginatedPosts, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get user info", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrUserNotFound.Wrap(err)
	}

	paginatedIds, err := us.idh.GetUserLikedPostIds(ctx, int64(user.Id), page, limit)
	if err != nil {
		us.l.Error("Failed to get liked post ids", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrInternal.Wrap(err)
	}

	if len(paginatedIds.Ids) == 0 {
		return &model.PaginatedPosts{Total: paginatedIds.Total, Page: page, Limit: limit, Posts: []model.Post{}}, nil
	}

	posts, err := us.pdh.FindPostsByIds(ctx, paginatedIds.Ids)
	if err != nil {
		us.l.Error("Failed to find posts by ids", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}

	postMap := make(map[int64]model.Post)
	for _, p := range posts {
		postMap[p.Id] = p
	}

	var orderedPosts []model.Post
	for _, id := range paginatedIds.Ids {
		if post, ok := postMap[id]; ok {
			orderedPosts = append(orderedPosts, post)
		}
	}

	likedIds, collectedIds, _ := us.idh.GetUserPostInteractionStatuses(ctx, int64(user.Id), paginatedIds.Ids)
	likedMap := make(map[int64]bool)
	collectedMap := make(map[int64]bool)
	for _, id := range likedIds {
		likedMap[id] = true
	}
	for _, id := range collectedIds {
		collectedMap[id] = true
	}

	details := us.ps.EnrichForSearcherWithStatuses(ctx, orderedPosts, studentId, likedMap, collectedMap)
	postsOut := make([]model.Post, 0, len(details))
	for _, d := range details {
		postsOut = append(postsOut, d.Post)
	}
	return &model.PaginatedPosts{Total: paginatedIds.Total, Page: page, Limit: limit, Posts: postsOut}, nil
}

func (us *UserService) LoadLikeAct(ctx context.Context, studentId string, page, limit int) (*model.PaginatedActivities, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		us.l.Error("Failed to get user info", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrUserNotFound.Wrap(err)
	}

	paginatedIds, err := us.idh.GetUserLikedActivityIds(ctx, int64(user.Id), page, limit)
	if err != nil {
		us.l.Error("Failed to get liked activity ids", zap.Error(err), zap.String("studentId", studentId))
		return nil, errs.ErrInternal.Wrap(err)
	}

	if len(paginatedIds.Ids) == 0 {
		return &model.PaginatedActivities{Total: paginatedIds.Total, Page: page, Limit: limit, Acts: []model.Activity{}}, nil
	}

	acts, err := us.adh.FindActsByIds(ctx, paginatedIds.Ids)
	if err != nil {
		us.l.Error("Failed to find activities by ids", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
	}

	actMap := make(map[int64]model.Activity)
	for _, act := range acts {
		actMap[act.Id] = act
	}

	var orderedActs []model.Activity
	for _, id := range paginatedIds.Ids {
		if act, ok := actMap[id]; ok {
			orderedActs = append(orderedActs, act)
		}
	}

	likedIds, collectedIds, _ := us.idh.GetUserActivityInteractionStatuses(ctx, int64(user.Id), paginatedIds.Ids)
	likedMap := make(map[int64]bool)
	collectedMap := make(map[int64]bool)
	for _, id := range likedIds {
		likedMap[id] = true
	}
	for _, id := range collectedIds {
		collectedMap[id] = true
	}

	details := us.as.EnrichForSearcherWithStatuses(ctx, orderedActs, studentId, likedMap, collectedMap)
	actsOut := make([]model.Activity, 0, len(details))
	for _, d := range details {
		actsOut = append(actsOut, d.Activity)
	}
	return &model.PaginatedActivities{Total: paginatedIds.Total, Page: page, Limit: limit, Acts: actsOut}, nil
}

func (us *UserService) VerifyUser(ctx context.Context, studentId string, realName string) (bool, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		if errors.Is(err, encrypt.ErrDecrypt) {
			us.l.Error("VerifyUser: real_name decrypt failed, data corruption", zap.Error(err), zap.String("studentId", studentId))
			return false, errs.ErrInternal.Wrap(err)
		}
		return false, errs.ErrUserNotFound
	}
	if string(user.RealName) != realName {
		return false, errs.ErrRealNameMismatch
	}
	return true, nil
}

func (us *UserService) EnrichPaginatedActivities(ctx context.Context, result *model.PaginatedActivities, studentId string) *model.PaginatedActivities {
	if len(result.Acts) == 0 {
		return &model.PaginatedActivities{Total: result.Total, Page: result.Page, Limit: result.Limit, Acts: []model.Activity{}}
	}
	details := us.as.EnrichForSearcher(ctx, result.Acts, studentId)
	actsOut := make([]model.Activity, 0, len(details))
	for _, d := range details {
		actsOut = append(actsOut, d.Activity)
	}
	return &model.PaginatedActivities{Total: result.Total, Page: result.Page, Limit: result.Limit, Acts: actsOut}
}

func (us *UserService) EnrichPaginatedPosts(ctx context.Context, result *model.PaginatedPosts, studentId string) *model.PaginatedPosts {
	if len(result.Posts) == 0 {
		return &model.PaginatedPosts{Total: result.Total, Page: result.Page, Limit: result.Limit, Posts: []model.Post{}}
	}
	details := us.ps.EnrichForSearcher(ctx, result.Posts, studentId)
	postsOut := make([]model.Post, 0, len(details))
	for _, d := range details {
		postsOut = append(postsOut, d.Post)
	}
	return &model.PaginatedPosts{Total: result.Total, Page: result.Page, Limit: result.Limit, Posts: postsOut}
}

func (us *UserService) EnrichActivitiesForResponse(ctx context.Context, acts []model.Activity, studentId string) []model.ActivityDetail {
	return us.as.EnrichForSearcher(ctx, acts, studentId)
}

func (us *UserService) EnrichPostsForResponse(ctx context.Context, posts []model.Post, studentId string) []model.PostDetail {
	return us.ps.EnrichForSearcher(ctx, posts, studentId)
}
