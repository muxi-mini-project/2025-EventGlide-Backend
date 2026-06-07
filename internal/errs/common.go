package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrParamInvalid = errorx.New(ParamInvalid, "参数错误")
	ErrNotFound     = errorx.New(NotFound, "资源不存在")
	ErrUnauthorized = errorx.New(Unauthorized, "未登录")
	ErrForbidden    = errorx.New(Forbidden, "无权限")
	ErrInternal     = errorx.New(InternalError, "服务器错误")
	ErrJWTExpired   = errorx.New(JWTExpired, "Token 已过期")
	ErrJWTInvalid   = errorx.New(JWTInvalid, "Token 无效")
	ErrTokenExpired = errorx.New(TokenExpired, "Token 已过期")
)