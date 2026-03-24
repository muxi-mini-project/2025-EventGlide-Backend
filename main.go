//go:generate swag init && wire gen .
package main

import (
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/raiki02/EG/internal/server"
	"github.com/raiki02/EG/pkg/ginx"
	"github.com/spf13/viper"
)

var (
	app *server.Server
	mu  sync.Mutex
)

// @Title EventGlide API
// @Description 校灵通 API 文档
// @verstion 1.0
func main() {
	ginx.InitValidation()
	app = InitApp()

	if err := app.Run(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	go hotLoad()

	select {}
}

func hotLoad() {
	viper.OnConfigChange(func(e fsnotify.Event) {
		if app.Shutdown != nil {
			app.Shutdown()
		}
		app = InitApp()
		if err := app.Run(); err != nil {
			log.Printf("Hot reload failed: %v", err)
		} else {
			log.Println("Hot reload successful")
		}
	})
	viper.WatchConfig()
}