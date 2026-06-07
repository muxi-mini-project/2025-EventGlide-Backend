package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrUserNotFound     = errorx.New(UserNotFound, "用户不存在")
	ErrUserBanned      = errorx.New(UserBanned, "用户已被封禁")
	ErrLoginFailed     = errorx.New(LoginFailed, "账号或密码错误")
	ErrNetworkError    = errorx.New(NetworkError, "网络异常")
	ErrRealNameMismatch = errorx.New(RealNameMismatch, "真实姓名与学号不匹配")
	ErrLoginInfoInvalid = errorx.New(LoginInfoInvalid, "登录信息无效")
)