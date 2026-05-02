package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	ingestionhttp "local/rag-project/internal/adapter/http/ingestion"
	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	raghttp "local/rag-project/internal/adapter/http/rag"
	settingshttp "local/rag-project/internal/adapter/http/settings"
	userhttp "local/rag-project/internal/adapter/http/user"
	corevector "local/rag-project/internal/app/rag/core/vector"
	ingestionbootstrap "local/rag-project/internal/bootstrap/ingestion"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	userbootstrap "local/rag-project/internal/bootstrap/user"
	"local/rag-project/internal/framework/config"
	fwlog "local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"
	umw "local/rag-project/internal/middleware"
)

func main() {
	_ = fwlog.Init()

	if err := config.LoadConfig(""); err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Get()
	port := 9090
	if cfg != nil && cfg.Server.Port != 0 {
		port = cfg.Server.Port
	}
	disableRocketMQ := shouldDisableRocketMQ()

	aiRuntime := infraai.NewRuntime()
	knowledgeRuntime, err := knowledgebootstrap.NewRuntime(context.Background(), knowledgebootstrap.RuntimeOptions{
		Config:          cfg,
		AIRuntime:       aiRuntime,
		DisableRocketMQ: disableRocketMQ,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init knowledge runtime failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := knowledgeRuntime.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close knowledge runtime failed: %v\n", err)
		}
	}()

	ingestionRuntime, err := ingestionbootstrap.NewRuntime(context.Background(), ingestionbootstrap.RuntimeOptions{
		Config: cfg,
		DB:     knowledgeRuntime.DB,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init ingestion runtime failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ingestionRuntime.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close ingestion runtime failed: %v\n", err)
		}
	}()

	var ragSearcher corevector.Searcher
	if knowledgeRuntime.VectorStore != nil {
		if searcher, ok := knowledgeRuntime.VectorStore.(corevector.Searcher); ok {
			ragSearcher = searcher
		}
	}
	ragRuntime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{
		Config:    cfg,
		DB:        knowledgeRuntime.DB,
		AIRuntime: aiRuntime,
		Searcher:  ragSearcher,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init rag runtime failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := ragRuntime.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close rag runtime failed: %v\n", err)
		}
	}()

	userRuntime, err := userbootstrap.NewRuntime(context.Background(), userbootstrap.RuntimeOptions{
		Config: cfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init user runtime failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := userRuntime.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close user runtime failed: %v\n", err)
		}
	}()

	r := gin.New()
	r.Use(umw.RequestIDMiddleware())
	r.Use(umw.ErrorHandlerMiddleware())
	r.Use(umw.UserContextMiddleware(userRuntime.LoadLoginUser, nil))

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	registerDebugAIRoutes(r, aiRuntime)
	registerKnowledgeRoutes(r, cfg, knowledgeRuntime)
	registerIngestionRoutes(r, cfg, ingestionRuntime)
	registerUserRoutes(r, cfg, userRuntime)
	registerRagRoutes(r, cfg, ragRuntime)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("starting server at %s\n", addr)
	if err := r.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "server exit: %v\n", err)
		os.Exit(1)
	}
}

func registerKnowledgeRoutes(r *gin.Engine, cfg *config.Config, runtime *knowledgebootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	routes := gin.IRouter(r)
	contextPath := ""
	if cfg != nil {
		contextPath = strings.Trim(strings.TrimSpace(cfg.Server.Servlet.ContextPath), "/")
	}
	if contextPath != "" {
		routes = r.Group("/" + contextPath)
	}
	admin := routes.Group("/")
	admin.Use(umw.RequireLogin(), umw.RequireRole("admin"))
	knowledgehttp.RegisterKnowledgeBaseRoutes(admin, runtime.BaseService)
	knowledgehttp.RegisterKnowledgeDocumentRoutes(admin, runtime.DocumentService)
	knowledgehttp.RegisterKnowledgeChunkRoutes(admin, runtime.ChunkService)
	settingshttp.RegisterRoutes(admin, cfg)
}

func registerIngestionRoutes(r *gin.Engine, cfg *config.Config, runtime *ingestionbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	routes := gin.IRouter(r)
	contextPath := ""
	if cfg != nil {
		contextPath = strings.Trim(strings.TrimSpace(cfg.Server.Servlet.ContextPath), "/")
	}
	if contextPath != "" {
		routes = r.Group("/" + contextPath)
	}
	admin := routes.Group("/")
	admin.Use(umw.RequireLogin(), umw.RequireRole("admin"))
	ingestionhttp.RegisterPipelineRoutes(admin, runtime.Pipeline)
	ingestionhttp.RegisterTaskRoutes(admin, runtime.Task)
}

func registerUserRoutes(r *gin.Engine, cfg *config.Config, runtime *userbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	routes := gin.IRouter(r)
	contextPath := ""
	if cfg != nil {
		contextPath = strings.Trim(strings.TrimSpace(cfg.Server.Servlet.ContextPath), "/")
	}
	if contextPath != "" {
		routes = r.Group("/" + contextPath)
	}
	userhttp.RegisterUserRoutes(routes, runtime.AuthService, runtime.UserService)
}

func registerRagRoutes(r *gin.Engine, cfg *config.Config, runtime *ragbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	routes := gin.IRouter(r)
	contextPath := ""
	if cfg != nil {
		contextPath = strings.Trim(strings.TrimSpace(cfg.Server.Servlet.ContextPath), "/")
	}
	if contextPath != "" {
		routes = r.Group("/" + contextPath)
	}
	protected := routes.Group("/")
	protected.Use(umw.RequireLogin())
	raghttp.RegisterRoutes(protected, runtime.Conversation, runtime.Message, runtime.Feedback, runtime.Chat, runtime.Trace)
}

func shouldDisableRocketMQ() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("DISABLE_ROCKETMQ")))
	return value == "1" || value == "true" || value == "yes"
}
