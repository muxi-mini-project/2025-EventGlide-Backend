package service

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/middleware"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	schoolReg = `学院：([^<]+)`
	schoolUrl = "https://bkzhjw.ccnu.edu.cn/jsxsd/framework/xsMainV_new_10511.htmlx?t1=1"
)

type UserServiceHdl interface {
	CreateUser(*gin.Context, string) error
	Login(*gin.Context, string, string) (*model.User, string, error)
	Logout(*gin.Context, string) error
	GetUserInfo(*gin.Context, string) (model.User, error)
	UpdateAvatar(*gin.Context, req.UserAvatarReq) error
	UpdateUsername(*gin.Context, string, string) error
	SearchUserAct(*gin.Context, string, string) ([]model.Activity, error)
	SearchUserPost(*gin.Context, string, string) ([]model.Post, error)
	GenQINIUToken(*gin.Context) string
	Like(*gin.Context, string, string) error
	Comment(*gin.Context, string, string) error
}

type UserService struct {
	udh  *dao.UserDao
	adh  *dao.ActDao
	pdh  *dao.PostDao
	cdh  *dao.CommentDao
	jwth *middleware.Jwt
	cSvc *ccnuService
	iuh  *ImgUploader
	as   *ActivityService
	ps   *PostService
	l    *zap.Logger
}

func NewUserService(udh *dao.UserDao, adh *dao.ActDao, pdh *dao.PostDao, cdh *dao.CommentDao, jwth *middleware.Jwt, cSvc *ccnuService, iuh *ImgUploader, as *ActivityService, ps *PostService, l *zap.Logger) *UserService {
	return &UserService{
		udh:  udh,
		adh:  adh,
		pdh:  pdh,
		cdh:  cdh,
		jwth: jwth,
		cSvc: cSvc,
		iuh:  iuh,
		as:   as,
		ps:   ps,
		l:    l.Named("user/service"),
	}
}

func (us *UserService) CreateUser(ctx *gin.Context, sid string, school string) error {
	user := &model.User{
		StudentID: sid,
		Name:      sid,
		//Avatar:    genRandomAvatar(ctx),
		Avatar:  viper.GetString("imgbed.defaultAvatar1"),
		School:  "华中师范大学",
		College: school,
	}
	err := us.udh.Create(ctx, user)
	if err != nil {
		return err
	}
	return nil
}

func (us *UserService) Login(ctx *gin.Context, studentId string, password string) (*model.User, string, error) {
	client, err := us.cSvc.Login(ctx, studentId, password)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		return nil, "", errors.New("登录失败")
	}

	if !us.udh.CheckUserExist(ctx, studentId) {
		school, err := us.cSvc.getWhichSchool(client)
		if err != nil {
			return nil, "", err
		}
		err = us.CreateUser(ctx, studentId, school)
		if err != nil {
			return nil, "", err
		}

	}
	token := us.jwth.GenToken(ctx, studentId)
	err = us.jwth.StoreInRedis(ctx, studentId, token)
	if err != nil {
		return nil, "", err
	}
	user, err := us.udh.GetUserInfo(ctx, studentId)
	return &user, token, nil
}

func (us *UserService) Logout(ctx *gin.Context, token string) error {
	err := us.jwth.ClearToken(ctx, token)
	if err != nil {
		return err
	}
	return nil
}

func (us *UserService) GetUserInfo(ctx *gin.Context, studentId string) (model.User, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		return model.User{}, err
	}
	return user, nil
}

func (us *UserService) UpdateAvatar(ctx *gin.Context, req req.UserAvatarReq, sid string) error {
	err := us.udh.UpdateAvatar(ctx, sid, req.AvatarUrl)
	if err != nil {
		return err
	}
	return nil
}

func (us *UserService) UpdateUsername(ctx *gin.Context, studentId string, name string) error {
	err := us.udh.UpdateUsername(ctx, studentId, name)
	if err != nil {
		return err
	}
	return nil
}

func (us *UserService) SearchUserAct(ctx *gin.Context, studentId string, keyword string) ([]resp.ListActivitiesResp, error) {
	acts, err := us.adh.FindActByUser(ctx, studentId, keyword)
	if err != nil {
		return nil, err
	}
	res := us.as.ToListResp(ctx, acts)
	return res, nil
}

func (us *UserService) SearchUserPost(ctx *gin.Context, studentId string, keyword string) ([]resp.ListPostsResp, error) {
	posts, err := us.pdh.FindPostByUser(ctx, studentId, keyword)
	if err != nil {
		return nil, err
	}
	res := us.ps.ToListResp(ctx, posts)
	return res, nil
}

func (us *UserService) GetChecking(ctx *gin.Context, studentId string) (resp.CheckingResp, error) {
	var res resp.CheckingResp
	acts, err := us.adh.GetChecking(ctx, studentId)
	if err != nil {
		return resp.CheckingResp{}, err
	}
	res.Acts = us.as.ToListResp(ctx, acts)

	posts, err := us.pdh.GetChecking(ctx, studentId)
	if err != nil {
		return resp.CheckingResp{}, err
	}
	res.Posts = us.ps.ToListResp(ctx, posts)

	return res, nil
}

//func genRandomAvatar(c *gin.Context) string {
//	avatars := []string{
//		viper.GetString("imgbed.defaultAvatar1"),
//		viper.GetString("imgbed.defaultAvatar2"),
//	}
//	n := rand.Intn(10)
//	if n == 9 {
//		return avatars[1]
//	} else {
//		return avatars[0]
//	}
//}

func (us *UserService) GenQINIUToken(ctx *gin.Context) resp.ImgBedResp {
	res := resp.ImgBedResp{
		AccessToken: us.iuh.GenQINIUToken(ctx),
		DomainName:  us.iuh.ImgUrl,
	}
	return res
}

func (us *UserService) LoadCollectAct(ctx *gin.Context, studentId string) ([]resp.ListActivitiesResp, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		return nil, err
	}
	var res []resp.ListActivitiesResp
	ActIDs := tools.StringToSlice(user.CollectAct)
	for _, id := range ActIDs {
		if id == "" {
			continue
		}
		acts, err := us.adh.FindActByBid(ctx, id)
		if err != nil {
			return nil, err
		}
		res = append(res, us.as.toListActResp(ctx, &acts))
	}
	return res, nil
}

func (us *UserService) LoadCollectPost(ctx *gin.Context, studentId string) ([]resp.ListPostsResp, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		return nil, err
	}
	var res []resp.ListPostsResp
	PostIDs := tools.StringToSlice(user.CollectPost)
	for _, id := range PostIDs {
		if id == "" {
			continue
		}
		posts, err := us.pdh.FindPostByBid(ctx, id)
		if err != nil {
			return nil, err
		}
		res = append(res, us.ps.toListPostResp(ctx, posts))
	}
	return res, nil
}

func (us *UserService) LoadLikePost(ctx *gin.Context, studentId string) ([]resp.ListPostsResp, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		return nil, err
	}
	var res []resp.ListPostsResp
	PostIDs := tools.StringToSlice(user.LikePost)
	for _, id := range PostIDs {
		if id == "" {
			continue
		}
		posts, err := us.pdh.FindPostByBid(ctx, id)
		if err != nil {
			return nil, err
		}
		res = append(res, us.ps.toListPostResp(ctx, posts))
	}
	return res, nil
}

func (us *UserService) LoadLikeAct(ctx *gin.Context, studentId string) ([]resp.ListActivitiesResp, error) {
	user, err := us.udh.GetUserInfo(ctx, studentId)
	if err != nil {
		return nil, err
	}
	var res []resp.ListActivitiesResp
	ActIDs := tools.StringToSlice(user.LikeAct)
	for _, id := range ActIDs {
		if id == "" {
			continue
		}
		acts, err := us.adh.FindActByBid(ctx, id)
		if err != nil {
			return nil, err
		}
		res = append(res, us.as.toListActResp(ctx, &acts))
	}
	return res, nil
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
	params, err := c.makeAccountPreflightRequest()
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("username", studentId)
	v.Set("password", password)
	v.Set("lt", params.lt)
	v.Set("execution", params.execution)
	v.Set("_eventId", params._eventId)
	v.Set("submit", params.submit)

	request, err := http.NewRequest("POST", "https://account.ccnu.edu.cn/cas/login;jsessionid="+params.JSESSIONID, strings.NewReader(v.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.109 Safari/537.36")
	request.WithContext(ctx)

	client := c.client()
	resp, err := client.Do(request)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return nil, errors.New("网络异常")
		}
		return nil, err
	}
	if len(resp.Header.Get("Set-Cookie")) == 0 {
		return nil, errors.New("学号或密码错误")
	}
	return client, nil
}

type accountRequestParams struct {
	lt         string
	execution  string
	_eventId   string
	submit     string
	JSESSIONID string
}

func (c *ccnuService) makeAccountPreflightRequest() (*accountRequestParams, error) {
	var JSESSIONID string
	var lt string
	var execution string
	var _eventId string

	params := &accountRequestParams{}

	// 初始化 http request
	request, err := http.NewRequest("GET", "https://account.ccnu.edu.cn/cas/login", nil)
	if err != nil {
		return params, err
	}

	// 发起请求
	resp, err := c.client().Do(request)
	if err != nil {
		return params, err
	}

	// 读取 Body
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	if err != nil {
		return params, err
	}

	// 获取 Cookie 中的 JSESSIONID
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "JSESSIONID" {
			JSESSIONID = cookie.Value
		}
	}

	if JSESSIONID == "" {
		return params, errors.New("Can not get JSESSIONID")
	}

	// 正则匹配 HTML 返回的表单字段
	ltReg := regexp.MustCompile("name=\"lt\".+value=\"(.+)\"")
	executionReg := regexp.MustCompile("name=\"execution\".+value=\"(.+)\"")
	_eventIdReg := regexp.MustCompile("name=\"_eventId\".+value=\"(.+)\"")

	bodyStr := string(body)

	ltArr := ltReg.FindStringSubmatch(bodyStr)
	if len(ltArr) != 2 {
		return params, errors.New("Can not get form paramater: lt")
	}
	lt = ltArr[1]

	execArr := executionReg.FindStringSubmatch(bodyStr)
	if len(execArr) != 2 {
		return params, errors.New("Can not get form paramater: execution")
	}
	execution = execArr[1]

	_eventIdArr := _eventIdReg.FindStringSubmatch(bodyStr)
	if len(_eventIdArr) != 2 {
		return params, errors.New("Can not get form paramater: _eventId")
	}
	_eventId = _eventIdArr[1]

	params.lt = lt
	params.execution = execution
	params._eventId = _eventId
	params.submit = "LOGIN"
	params.JSESSIONID = JSESSIONID

	return params, nil
}

func (c *ccnuService) getWhichSchool(client *http.Client) (string, error) {
	resp, err := client.Get(schoolUrl)
	if err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", err
	}
	bodyStr := string(body)

	reg := regexp.MustCompile(schoolReg)
	matches := reg.FindStringSubmatch(bodyStr)
	if len(matches) != 2 {
		return "", errors.New("Can not get school info")
	}
	return matches[1], nil
}
