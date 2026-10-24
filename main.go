//go:generate swag init && wire gen .
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/raiki02/EG/docs"
	"github.com/raiki02/EG/pkg/ginx"
)

// @title EventGlide API
// @version 1.0
// @description 校灵通 API 文档
func main() {
	// 先注册信号再启动：启动阶段收到 SIGTERM 也能走优雅关闭；stop() 后恢复默认处理，
	// 二次信号可直接强杀卡住的进程。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ginx.InitValidation()
	app := InitApp()

	if err := app.Run(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	<-ctx.Done()
	stop()
	app.Shutdown()
}
