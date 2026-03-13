package ginx

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/raiki02/EG/api/resp"
	"net/http"
)

type userClaims struct{}

var (
	UserClaimsKey userClaims
)

func WrapRequest[request any](fn func(*gin.Context, request) (resp.Resp, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var (
			req request
			err error
		)

		if err = bind(ctx, &req); err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(err))
			return
		}

		res, err := fn(ctx, req)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(err))
			return
		}

		ctx.JSON(res.Code, res)
	}
}

func WrapRequestWithClaims[request any](fn func(*gin.Context, request, jwt.RegisteredClaims) (resp.Resp, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var (
			req request
			err error
		)

		uk, ok := ctx.Get(UserClaimsKey)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(errors.New("user key not found")))
			return
		}

		claim, ok := uk.(jwt.RegisteredClaims)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(errors.New("user claim is not jwt.RegisteredClaims")))
			return
		}

		if err = bind(ctx, &req); err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(err))
			return
		}

		res, err := fn(ctx, req, claim)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(err))
			return
		}

		ctx.JSON(res.Code, res)
	}
}

func Wrap(fn func(*gin.Context) (resp.Resp, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		res, err := fn(ctx)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(err))
			return
		}

		ctx.JSON(res.Code, res)
	}
}

func WrapWithClaims(fn func(*gin.Context, jwt.RegisteredClaims) (resp.Resp, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		uk, ok := ctx.Get(UserClaimsKey)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(errors.New("user claim is not jwt.RegisteredClaims")))
			return
		}

		claim, ok := uk.(jwt.RegisteredClaims)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(errors.New("user claim is not jwt.RegisteredClaims")))
			return
		}
		res, err := fn(ctx, claim)

		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, ReturnOnlyErrorResp(err))
			return
		}

		ctx.JSON(res.Code, res)
	}
}

func bind(ctx *gin.Context, req any) (err error) {
	if err = ctx.ShouldBindUri(req); err != nil {
		return
	}

	if ctx.Request.Method == http.MethodGet {
		err = ctx.ShouldBindQuery(req)
	} else {
		err = ctx.ShouldBind(req)
	}
	if err != nil {
		return
	}

	if err = validateRequest(req); err != nil {
		return
	}

	return nil
}

func ReturnError(err error) (resp.Resp, error) {
	return resp.Resp{
		Msg:  err.Error(),
		Code: 500,
	}, err
}

func ReturnOnlyErrorResp(err error) resp.Resp {
	return resp.Resp{
		Code: 500,
		Msg:  err.Error(),
	}
}

func ReturnSuccess(data any) (resp.Resp, error) {
	return resp.Resp{
		Code: 200,
		Msg:  "success",
		Data: data,
	}, nil
}
