package middleware

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/pkg/ginx"
	"github.com/raiki02/EG/tools"
	"github.com/redis/go-redis/v9"
)

type JwtHdl interface {
	GenToken(*gin.Context, string) string
	StoreInRedis(*gin.Context, string, string) error
	CheckToken(*gin.Context, string) error
	ClearToken(*gin.Context, string) error
}
type Jwt struct {
	rdb    *redis.Client
	cfg *config.Conf
	jwtKey []byte
}

func NewJwt(rdb *redis.Client, cfg *config.Conf) *Jwt {
	jwtKey := cfg.JWT.Key
	return &Jwt{
		jwtKey: []byte(jwtKey),
		cfg:cfg,
		rdb:    rdb,
	}
}

func (c *Jwt) GenToken(ctx *gin.Context,  sid string) string {
	claims := jwt.RegisteredClaims{
		ID:        uuid.New().String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(setTTL(c.cfg))),
		Subject:   sid,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t, err := token.SignedString(c.jwtKey)
	if err != nil {
		return ""
	}
	return t
}

func (c *Jwt) StoreInRedis(ctx *gin.Context,sid string, token string) error {
	id := c.parseTokenId(token)
	key := "token:" + id
	err := c.rdb.Set(ctx, key, sid, setTTL(c.cfg)).Err()
	if err != nil {
		return err
	}
	return nil
}

func (c *Jwt) CheckToken(ctx *gin.Context, token string) error {
	id := c.parseTokenId(token)
	if id == "" {
		return errors.New("token is invalid")
	}
	id = "token:" + id
	_, err := c.rdb.Get(ctx, id).Result()
	if err != nil {
		return err
	}
	return nil
}

func (c *Jwt) ClearToken(ctx *gin.Context, token string) error {
	id := c.parseTokenId(token)
	id = "token:" + id
	err := c.rdb.Del(ctx, id).Err()
	if err != nil {
		return err
	}
	return nil
}

func (c *Jwt) parseTokenId(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer") {
		_, token, _ = strings.Cut(token, " ")
	}
	t, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return c.jwtKey, nil
	})
	if err != nil {
		return ""
	}
	if c, ok := t.Claims.(*jwt.RegisteredClaims); ok && t.Valid {
		return c.ID
	}
	return ""
}

func setTTL(cfg *config.Conf) time.Duration {
	ttl := cfg.JWT.Ttl
	fmt.Println(ttl)
	return time.Second * time.Duration(ttl)
}

func (c *Jwt) WrapCheckToken() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader("Authorization")
		if token == "" {
			ctx.JSON(401, tools.ReturnMSG(ctx, "token is empty", nil))
			ctx.Abort()
			return
		}
		err := c.CheckToken(ctx, token)
		if err != nil {
			ctx.JSON(401, tools.ReturnMSG(ctx, "token is invalid", nil))
			ctx.Abort()
			return
		}
		ctx.Set("studentid", c.parseSid(token))
		claims := c.parseToken(token)
		ctx.Set(ginx.UserClaimsKey, *claims)
		ctx.Next()
	}
}

func (c *Jwt) parseSid(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer") {
		_, token, _ = strings.Cut(token, " ")
	}
	t, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return c.jwtKey, nil
	})
	if err != nil {
		return ""
	}
	if c, ok := t.Claims.(*jwt.RegisteredClaims); ok && t.Valid {
		return c.Subject
	}
	return ""
}

func (c *Jwt) parseToken(token string) *jwt.RegisteredClaims {
	if token == "" {
		return nil
	}
	if strings.HasPrefix(token, "Bearer") {
		_, token, _ = strings.Cut(token, " ")
	}
	t, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return c.jwtKey, nil
	})
	if err != nil {
		return nil
	}
	if c, ok := t.Claims.(*jwt.RegisteredClaims); ok && t.Valid {
		return c
	}
	return nil
}
