package ioc

import "github.com/gin-gonic/gin"

func InitGinHandler() *gin.Engine {
	e := gin.New()
	e.Use(
		gin.LoggerWithConfig(gin.LoggerConfig{
			SkipPaths: []string{"/metrics", "/healthz"},
		}),
		gin.Recovery(),
	)
	return e
}
