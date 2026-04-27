package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/contextx"
	fwlog "local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"

	umw "local/rag-project/internal/middleware"
)

func main() {
	// init logger
	_ = fwlog.Init()

	// 加载配置（默认从 ./configs/application.yaml）
	if err := config.LoadConfig(""); err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Get()
	port := 9090
	if cfg != nil && cfg.Server.Port != 0 {
		port = cfg.Server.Port
	}

	runtime := infraai.NewRuntime()

	r := gin.New()

	// 挂载全局异常处理
	r.Use(umw.RequestIDMiddleware())
	r.Use(umw.ErrorHandlerMiddleware())

	// 示例内存用户 loader
	loader := func(loginId string) (*contextx.LoginUser, error) {
		if loginId == "1" {
			return &contextx.LoginUser{
				UserID:   "1",
				Username: "alice",
				Role:     "user",
				Avatar:   "https://avatars.githubusercontent.com/u/583231?v=4",
			}, nil
		}
		return nil, nil
	}

	r.Use(umw.UserContextMiddleware(loader, nil))

	r.GET("/ping", func(c *gin.Context) {
		u := contextx.Get(c)
		if u != nil {
			c.JSON(200, gin.H{"message": "pong", "user": u.Username})
			return
		}
		c.JSON(200, gin.H{"message": "pong"})
	})

	registerDebugAIRoutes(r, runtime)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("starting server at %s\n", addr)
	if err := r.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "server exit: %v\n", err)
		os.Exit(1)
	}
}
