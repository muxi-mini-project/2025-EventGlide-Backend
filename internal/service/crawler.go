package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

// 一站式登录
type ccnuService struct {
	timeout time.Duration
	proxy   *shenlongProxyProvider
	l       *zap.Logger
}

type StudentType uint8

const (
	Unknown StudentType = iota
	PostGraduate
	UnderGraduate
)

func ParseStudentType(studentID string) StudentType {
	if len(studentID) <= 4 {
		return Unknown
	}

	switch studentID[4] {
	case '0', '1':
		return PostGraduate
	case '2':
		return UnderGraduate
	default:
		return Unknown
	}
}

type StudentCrawler interface {
	Login(ctx context.Context, studentID, password string) (*http.Client, error)
	SupportsUserInfo(studentID string) bool
	GetNameAndDepartment(ctx context.Context, studentID string, client *http.Client) (string, string, error)
}

var _ StudentCrawler = (*ccnuService)(nil)

const (
	ccnuAccountHost      = "account.ccnu.edu.cn"
	ccnuPostgraduateHost = "grd.ccnu.edu.cn"
)

type loginProxyError struct {
	err error
}

func (e *loginProxyError) Error() string {
	return fmt.Sprintf("login proxy failed: %v", e.err)
}

func (e *loginProxyError) Unwrap() error {
	return e.err
}

func NewCCNUService(cfg *config.Conf, l *logger.LoggerSet) *ccnuService {
	return &ccnuService{
		timeout: time.Second * 15,
		proxy:   newShenlongProxyProvider(cfg.ShenlongConf),
		l:       l.User.Named("ccnu"),
	}
}

func (c *ccnuService) Login(ctx context.Context, studentId string, password string) (*http.Client, error) {
	var login func(context.Context, string, string, bool) (*http.Client, error)
	switch ParseStudentType(studentId) {
	case UnderGraduate:
		login = c.loginUndergraduateClient
	case PostGraduate:
		login = c.loginPostgraduateClient
	default:
		return nil, errs.ErrLoginInfoInvalid
	}

	client, err := login(ctx, studentId, password, true)
	if err == nil {
		return client, nil
	}

	if c.proxy == nil || !shouldRetryLoginDirect(err) {
		return nil, err
	}

	c.proxy.invalidate()
	c.l.Warn("proxy login failed, retrying directly", zap.Error(err))
	return login(ctx, studentId, password, false)
}

// SupportsUserInfo reports whether the student type has a matching user-info crawler.
func (c *ccnuService) SupportsUserInfo(studentID string) bool {
	switch ParseStudentType(studentID) {
	case UnderGraduate, PostGraduate:
		return true
	default:
		return false
	}
}

func (c *ccnuService) client(ctx context.Context, useProxy bool) (*http.Client, error) {
	j, _ := cookiejar.New(&cookiejar.Options{})
	var proxyURL *url.URL
	if useProxy && c.proxy != nil {
		var err error
		proxyURL, err = c.proxy.proxyURL(ctx)
		if err != nil {
			return nil, &loginProxyError{err: err}
		}
	}

	return &http.Client{
		Transport: newCCNUTransport(proxyURL),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
		Jar:     j,
		Timeout: c.timeout,
	}, nil
}

func newCCNUTransport(proxyURL *url.URL) *http.Transport {
	transport := &http.Transport{
		MaxIdleConnsPerHost:   10,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	if proxyURL == nil {
		return transport
	}

	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		if strings.EqualFold(host, ccnuAccountHost) || strings.EqualFold(host, ccnuPostgraduateHost) {
			return proxyURL, nil
		}
		return nil, nil
	}
	return transport
}

func shouldRetryLoginDirect(err error) bool {
	var proxyErr *loginProxyError
	var netErr net.Error
	return errors.As(err, &proxyErr) ||
		errors.As(err, &netErr) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, errs.ErrNetworkError) ||
		errors.Is(err, errs.ErrLoginInfoInvalid)
}

// 本科生登录
func (c *ccnuService) loginUndergraduateClient(ctx context.Context, studentId string, password string, useProxy bool) (*http.Client, error) {
	client, params, err := c.makeAccountPreflightRequest(ctx, useProxy)
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

	request, err := http.NewRequestWithContext(ctx, "POST", "https://account.ccnu.edu.cn/cas/login;jsessionid="+params.JSESSIONID, strings.NewReader(v.Encode()))
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

func (c *ccnuService) GetNameAndDepartment(ctx context.Context, studentID string, client *http.Client) (string, string, error) {
	switch ParseStudentType(studentID) {
	case UnderGraduate:
		return c.getUndergraduateNameAndDepartment(ctx, client)
	case PostGraduate:
		return newPostgraduateCrawler(client).GetNameAndDepartment(ctx)
	default:
		return "", "", errs.ErrLoginInfoInvalid
	}
}

func (c *ccnuService) getUndergraduateNameAndDepartment(ctx context.Context, client *http.Client) (string, string, error) {
	url1 := "https://account.ccnu.edu.cn/cas/login?service=" + url.QueryEscape("https://bkzhjw.ccnu.edu.cn/jsxsd/framework/xsMainV_new_10511.htmlx?t1=1")

	req1, err := http.NewRequestWithContext(ctx, http.MethodGet, url1, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := client.Do(req1)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	url2 := "https://account.ccnu.edu.cn/cas/login?service=" + url.QueryEscape("https://bkzhjw.ccnu.edu.cn/jsxsd/framework/xsMainV_new_10511.htmlx?t1=1")

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, url2, nil)
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

func (c *ccnuService) makeAccountPreflightRequest(ctx context.Context, useProxy bool) (*http.Client, *accountRequestParams, error) {
	var JSESSIONID string
	var lt string
	var execution string
	var _eventId string
	params := &accountRequestParams{}
	client, err := c.client(ctx, useProxy)
	if err != nil {
		return nil, params, err
	}

	// 初始化 http request
	request, err := http.NewRequestWithContext(ctx, "GET", "https://account.ccnu.edu.cn/cas/login", nil)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for i := 0; i < 3; i++ {
		realName, college, err := us.cSvc.GetNameAndDepartment(ctx, studentID, client)

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
				us.l.Info("user info updated", zap.String("student_id", studentID), zap.String("college", college))
				return
			}
		}

		us.l.Warn("load user info failed", zap.String("student_id", studentID), zap.Int("retry", i+1), zap.Error(err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
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
		if department == "" && isDepartmentText(text) {
			department = detailValue(text)
		}
	})

	return name, department, nil
}

func isDepartmentText(text string) bool {
	return strings.Contains(text, "学院") ||
		strings.Contains(text, "院系") ||
		strings.Contains(text, "院：") ||
		strings.Contains(text, "院:")
}

func detailValue(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\u00a0", ""))
	if pos := strings.Index(text, "："); pos >= 0 {
		return strings.TrimSpace(text[pos+len("："):])
	}
	if pos := strings.Index(text, ":"); pos >= 0 {
		return strings.TrimSpace(text[pos+1:])
	}
	return ""
}

// 研究生登录
const (
	postgraduateURL         = "https://grd.ccnu.edu.cn"
	publicKeyURL            = postgraduateURL + "/yjsxt/xtgl/login_getPublicKey.html"
	loginPostgraduateURL    = postgraduateURL + "/yjsxt/xtgl/login_slogin.html"
	postgraduateUserInfoURL = postgraduateURL + "/yjsxt/xtgl/index_cxUserInfo.html?gnmkdm=index"
	postgraduateMenuReferer = postgraduateURL + "/yjsxt/xtgl/index_initMenu.html"
)

type postgraduateCrawler struct {
	client *http.Client
}

func newPostgraduateCrawler(client *http.Client) *postgraduateCrawler {
	return &postgraduateCrawler{client: client}
}

func (c *postgraduateCrawler) GetNameAndDepartment(ctx context.Context) (string, string, error) {
	form := url.Values{}
	form.Set("localeKey", "zh_CN")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		postgraduateUserInfoURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", "", fmt.Errorf("postgraduate: create user info request failed: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Origin", postgraduateURL)
	req.Header.Set("Referer", postgraduateMenuReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("postgraduate: send user info request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponse(resp)
	if err != nil {
		return "", "", fmt.Errorf("postgraduate: read user info response failed: %w", err)
	}

	var data postgraduateUserInfoResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", fmt.Errorf("postgraduate: decode user info response failed: %w", err)
	}
	if data.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("postgraduate: user info status code %d", data.StatusCode)
	}

	name := strings.TrimSpace(data.Data.Name)
	department := strings.TrimSpace(data.Data.Department)
	if name == "" || department == "" {
		return "", "", errors.New("postgraduate: name or department missing")
	}
	return name, department, nil
}

type postgraduateUserInfoResponse struct {
	Data struct {
		Name       string `json:"xm"`
		Department string `json:"jgmc"`
	} `json:"data"`
	StatusCode int `json:"statusCode"`
}

// 1. 获取 RSA 公钥
func (c *postgraduateCrawler) FetchPublicKey(ctx context.Context) (*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", publicKeyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("postgraduate: create public key request failed: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", postgraduateURL+"/yjsxt/")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("postgraduate: send public key request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("postgraduate: read public key response failed: %w", err)
	}
	var data rsaPublicKeyResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("postgraduate: decode public key response failed: %w", err)
	}

	pubKey, err := parseRSAPublicKey(data.Modulus, data.Exponent)
	if err != nil {
		return nil, fmt.Errorf("postgraduate: parse public key failed: %w", err)
	}

	return pubKey, nil
}

// 2. 登录研究生系统
func (c *postgraduateCrawler) LoginPostgraduateSystem(
	ctx context.Context,
	username,
	password string,
	pubKey *rsa.PublicKey,
) error {

	encPwd, err := encryptPasswordJSStyle(password, pubKey)
	if err != nil {
		return fmt.Errorf("postgraduate: encrypt password failed: %w", err)
	}

	form := url.Values{}
	form.Set("csrftoken", "")
	form.Set("yhm", username)
	form.Set("mm", encPwd)

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		loginPostgraduateURL,
		bytes.NewBufferString(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("postgraduate: create login request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", postgraduateURL+"/yjsxt/")
	req.Header.Set("Origin", postgraduateURL)
	req.Header.Set("Host", "grd.ccnu.edu.cn")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("postgraduate: send login request failed: %w", err)
	}
	defer resp.Body.Close()

	if _, err := readResponse(resp); err != nil {
		return fmt.Errorf("postgraduate: read login response failed: %w", err)
	}
	if !hasPostgraduateSession(c.client) {
		return errs.ErrLoginFailed
	}

	return nil
}

func hasPostgraduateSession(client *http.Client) bool {
	if client == nil || client.Jar == nil {
		return false
	}

	loginURL, err := url.Parse(loginPostgraduateURL)
	if err != nil {
		return false
	}

	var hasJSESSIONID, hasRoute bool
	for _, cookie := range client.Jar.Cookies(loginURL) {
		switch cookie.Name {
		case "JSESSIONID":
			hasJSESSIONID = cookie.Value != ""
		case "route":
			hasRoute = cookie.Value != ""
		}
	}
	return hasJSESSIONID && hasRoute
}

type rsaPublicKeyResponse struct {
	Modulus  string `json:"modulus"`
	Exponent string `json:"exponent"`
}

func parseRSAPublicKey(modBase64, expBase64 string) (*rsa.PublicKey, error) {
	modBytes, err := base64.StdEncoding.DecodeString(modBase64)
	if err != nil {
		return nil, fmt.Errorf("rsa: decode modulus failed: %w", err)
	}

	expBytes, err := base64.StdEncoding.DecodeString(expBase64)
	if err != nil {
		return nil, fmt.Errorf("rsa: decode exponent failed: %w", err)
	}

	modulus := new(big.Int).SetBytes(modBytes)
	exponent := new(big.Int).SetBytes(expBytes)

	return &rsa.PublicKey{
		N: modulus,
		E: int(exponent.Int64()),
	}, nil
}

func encryptPasswordJSStyle(password string, pubKey *rsa.PublicKey) (string, error) {
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, pubKey, []byte(password))
	if err != nil {
		return "", fmt.Errorf("rsa: encrypt password failed: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func (c *ccnuService) loginPostgraduateClient(ctx context.Context, studentID string, password string, useProxy bool) (*http.Client, error) {
	client, err := c.client(ctx, useProxy)
	if err != nil {
		return nil, err
	}

	crawler := newPostgraduateCrawler(client)
	publicKey, err := crawler.FetchPublicKey(ctx)
	if err != nil {
		return nil, err
	}
	if err := crawler.LoginPostgraduateSystem(ctx, studentID, password, publicKey); err != nil {
		return nil, err
	}

	return client, nil
}

func readResponse(resp *http.Response) ([]byte, error) {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// 神龙代理
type shenlongProxyProvider struct {
	conf      config.ShenlongConf
	mu        sync.Mutex
	proxyAddr string
	expiresAt time.Time
}

func (p *shenlongProxyProvider) invalidate() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.proxyAddr = ""
	p.expiresAt = time.Time{}
	p.mu.Unlock()
}

func newShenlongProxyProvider(conf config.ShenlongConf) *shenlongProxyProvider {
	if strings.TrimSpace(conf.API) == "" {
		return nil
	}
	return &shenlongProxyProvider{conf: conf}
}

func (p *shenlongProxyProvider) proxyURL(ctx context.Context) (*url.URL, error) {
	now := time.Now()

	p.mu.Lock()
	if p.proxyAddr != "" && now.Before(p.expiresAt) {
		addr := p.proxyAddr
		p.mu.Unlock()
		return url.Parse(addr)
	}
	p.mu.Unlock()

	addr, err := p.fetchProxy(ctx)
	if err != nil {
		return nil, err
	}

	interval := p.conf.Interval
	if interval <= 0 {
		interval = 60
	}

	p.mu.Lock()
	p.proxyAddr = addr
	p.expiresAt = time.Now().Add(time.Duration(interval) * time.Second)
	p.mu.Unlock()

	return url.Parse(addr)
}

func (p *shenlongProxyProvider) fetchProxy(ctx context.Context) (string, error) {
	retry := p.conf.Retry
	if retry <= 0 {
		retry = 1
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for i := 0; i < retry; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.conf.API, nil)
		if err != nil {
			return "", err
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("shenlong proxy api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		proxyAddr, err := p.formatProxyAddr(string(body))
		if err != nil {
			lastErr = err
			continue
		}
		return proxyAddr, nil
	}

	if lastErr == nil {
		lastErr = errors.New("shenlong proxy api returned no proxy")
	}
	return "", lastErr
}

func (p *shenlongProxyProvider) formatProxyAddr(raw string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return "", errors.New("empty proxy address")
	}

	addr := fields[0]
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}

	proxyURL, err := url.Parse(addr)
	if err != nil {
		return "", err
	}
	if p.conf.Username != "" || p.conf.Password != "" {
		proxyURL.User = url.UserPassword(p.conf.Username, p.conf.Password)
	}

	return proxyURL.String(), nil
}
