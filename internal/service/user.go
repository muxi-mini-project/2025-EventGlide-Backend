package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
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
	cSvc *ccnuService
	iuh  *ImgUploader
	as   *ActivityService
	ps   *PostService
	l    *zap.Logger
	cfg  *config.Conf
}

func NewUserService(udh *repo.UserRepo, adh *repo.ActivityRepo, pdh *repo.PostRepo, ih *repo.InteractionRepo, jwth *middleware.Jwt, cSvc *ccnuService, iuh *ImgUploader, as *ActivityService, ps *PostService, l *logger.LoggerSet, cfg *config.Conf) *UserService {
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
		Name:      sid,
		RealName:  name,
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
		return nil, "", errs.ErrInternal.Wrap(err)
	}
	if client == nil {
		return nil, "", errs.ErrLoginFailed
	}

	name, department, err := us.cSvc.getNameAndDepartment(client)
	if err != nil {
		us.l.Warn("get user info failed", zap.Error(err))
		name = ""
		department = ""
	}

	if !us.udh.CheckUserExist(ctx, studentId) {
		if name == "" || department == "" {
			go us.loadUserInfoAsync(client, studentId)
		}

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
		return nil, "", errs.ErrUserNotFound.Wrap(err)
	}

	if user.RealName == "" || user.College == "" {
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
		return false, errs.ErrUserNotFound
	}
	if user.RealName != realName {
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

//---一站式账号登录------------------------------------------------------------

type ccnuService struct {
	timeout time.Duration
}

func NewCCNUService() *ccnuService {
	return &ccnuService{
		timeout: time.Second * 15,
	}
}

func (c *ccnuService) Login(ctx context.Context, studentId string, password string) (*http.Client, error) {
	var (
		client *http.Client
		err    error
	)
	client, err = c.loginUndergraduateClient(ctx, studentId, password)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (c *ccnuService) client() *http.Client {
	j, _ := cookiejar.New(&cookiejar.Options{})
	return &http.Client{
		Transport: nil,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
		Jar:     j,
		Timeout: c.timeout,
	}
}

func (c *ccnuService) loginUndergraduateClient(ctx context.Context, studentId string, password string) (*http.Client, error) {
	client, params, err := c.makeAccountPreflightRequest()
	if err != nil {
		return nil, err
	}

	id := tools.RandomMD5()
	v := url.Values{}
	v.Set("username", studentId)
	v.Set("password", password)
	v.Set("lt", params.lt)
	v.Set("execution", params.execution)
	v.Set("_eventId", params._eventId)
	v.Set("submit", params.submit)

	v.Set("visitorId1", id)
	v.Set("visitorId", id)

	request, err := http.NewRequest("POST", "https://account.ccnu.edu.cn/cas/login;jsessionid="+params.JSESSIONID, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.109 Safari/537.36")

	resp, err := client.Do(request)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return nil, errs.ErrNetworkError
		}
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "有误") {
		return client, errs.ErrLoginFailed
	}
	return client, nil
}

func (c *ccnuService) getNameAndDepartment(client *http.Client) (string, string, error) {
	url1 := "https://account.ccnu.edu.cn/cas/login?service=" + url.QueryEscape("https://bkzhjw.ccnu.edu.cn/jsxsd/framework/xsMainV_new_10511.htmlx?t1=1")

	req1, err := http.NewRequest("GET", url1, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := client.Do(req1)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	url2 := "https://account.ccnu.edu.cn/cas/login?service=" + url.QueryEscape("https://bkzhjw.ccnu.edu.cn/jsxsd/framework/xsMainV_new_10511.htmlx?t1=1")

	req2, err := http.NewRequest("GET", url2, nil)
	if err != nil {
		return "", "", err
	}

	resp2, err := client.Do(req2)
	if err != nil {
		return "", "", err
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)

	name, department, err := parseInfo(string(body2))
	if err != nil {
		return "", "", err
	}

	return name, department, nil
}

type accountRequestParams struct {
	lt         string
	execution  string
	_eventId   string
	submit     string
	JSESSIONID string
}

func (c *ccnuService) makeAccountPreflightRequest() (*http.Client, *accountRequestParams, error) {
	var JSESSIONID string
	var lt string
	var execution string
	var _eventId string
	client := c.client()

	params := &accountRequestParams{}

	// 初始化 http request
	request, err := http.NewRequest("GET", "https://account.ccnu.edu.cn/cas/login", nil)
	if err != nil {
		return client, params, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.109 Safari/537.36")

	// 发起请求
	resp, err := client.Do(request)
	if err != nil {
		return client, params, err
	}

	// 读取 Body
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	if err != nil {
		return client, params, err
	}

	// 获取 Cookie 中的 JSESSIONID
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "JSESSIONID" {
			JSESSIONID = cookie.Value
		}
	}

	if JSESSIONID == "" {
		return client, params, errs.ErrLoginInfoInvalid
	}

	// 正则匹配 HTML 返回的表单字段
	ltReg := regexp.MustCompile("name=\"lt\".+value=\"(.+)\"")
	executionReg := regexp.MustCompile("name=\"execution\".+value=\"(.+)\"")
	_eventIdReg := regexp.MustCompile("name=\"_eventId\".+value=\"(.+)\"")

	bodyStr := string(body)

	ltArr := ltReg.FindStringSubmatch(bodyStr)
	if len(ltArr) != 2 {
		return client, params, errs.ErrLoginInfoInvalid
	}
	lt = ltArr[1]

	execArr := executionReg.FindStringSubmatch(bodyStr)
	if len(execArr) != 2 {
		return client, params, errs.ErrLoginInfoInvalid
	}
	execution = execArr[1]

	_eventIdArr := _eventIdReg.FindStringSubmatch(bodyStr)
	if len(_eventIdArr) != 2 {
		return client, params, errs.ErrLoginInfoInvalid
	}
	_eventId = _eventIdArr[1]

	params.lt = lt
	params.execution = execution
	params._eventId = _eventId
	params.submit = "LOGIN"
	params.JSESSIONID = JSESSIONID

	return client, params, nil
}

func (us *UserService) loadUserInfoAsync(client *http.Client, studentID string) {
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		realName, college, err := us.cSvc.getNameAndDepartment(client)

		if err == nil {
			updated := false

			if realName != "" {
				if err = us.udh.UpdateRealName(ctx, studentID, realName); err == nil {
					updated = true
				}
			}

			if college != "" {
				if err = us.udh.UpdateCollege(ctx, studentID, college); err == nil {
					updated = true
				}
			}

			if updated {
				us.l.Info("user info updated", zap.String("student_id", studentID), zap.String("realName", realName), zap.String("college", college))
				return
			}
		}

		us.l.Warn("load user info failed", zap.String("student_id", studentID), zap.Int("retry", i+1), zap.Error(err))
		time.Sleep(30 * time.Second)
	}
}

func parseInfo(html string) (name, department string, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", "", err
	}

	nameText := strings.TrimSpace(
		doc.Find(".infoContentTitle").First().Text(),
	)

	if idx := strings.Index(nameText, "-"); idx > 0 {
		name = nameText[:idx]
	} else {
		name = nameText
	}

	doc.Find(".qz-detailtext").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())

		if strings.Contains(text, "院：") {
			if pos := strings.Index(text, "："); pos >= 0 {
				department = strings.TrimSpace(text[pos+len("："):])
			}
		}
	})

	return
}
