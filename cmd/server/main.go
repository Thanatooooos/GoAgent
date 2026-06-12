package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	ingestionhttp "local/rag-project/internal/adapter/http/ingestion"
	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	raghttp "local/rag-project/internal/adapter/http/rag"
	settingshttp "local/rag-project/internal/adapter/http/settings"
	userhttp "local/rag-project/internal/adapter/http/user"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
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
	if err := fwlog.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "init log failed: %v\n", err)
		os.Exit(1)
	}

	if err := config.LoadConfig(""); err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Get()
	port := 9090
	if cfg != nil && cfg.Server.Port != 0 {
		port = cfg.Server.Port
	}
	// 先建库并执行所有迁移，确保表在 runtime 启动前已就绪。
	initDB, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init db failed: %v\n", err)
		os.Exit(1)
	}
	if err := postgresrepo.RunMigrations(initDB); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations failed: %v\n", err)
		os.Exit(1)
	}
	if sqlDB, err := initDB.DB(); err == nil {
		sqlDB.Close()
	}

	aiRuntime := infraai.NewRuntime()
	knowledgeRuntime, err := knowledgebootstrap.NewRuntime(context.Background(), knowledgebootstrap.RuntimeOptions{
		Config:    cfg,
		AIRuntime: aiRuntime,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init knowledge runtime failed: %v\n", err)
		os.Exit(1)
	}

	ingestionRuntime, err := ingestionbootstrap.NewRuntime(context.Background(), ingestionbootstrap.RuntimeOptions{
		Config: cfg,
		DB:     knowledgeRuntime.DB,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init ingestion runtime failed: %v\n", err)
		os.Exit(1)
	}
	if knowledgeRuntime.DocumentService != nil && ingestionRuntime.Task != nil {
		knowledgeRuntime.DocumentService.SetIngestionTaskCreator(knowledgebootstrap.NewIngestionTaskCreator(ingestionRuntime.Task))
		knowledgeRuntime.DocumentService.SetIngestionTaskReader(knowledgebootstrap.NewIngestionTaskReader(ingestionRuntime.Task))
	}
	if knowledgeRuntime.DocumentService != nil && ingestionRuntime.Metrics != nil {
		knowledgeRuntime.DocumentService.SetIngestionReconcileRecorder(
			knowledgebootstrap.NewIngestionReconcileRecorder(ingestionRuntime.Metrics),
		)
	}
	if ingestionRuntime.Executor != nil && knowledgeRuntime.DocumentService != nil {
		ingestionRuntime.Executor.SetTaskObserver(ingestionservice.NewMultiTaskObserver(
			ingestionRuntime.Executor.Observer(),
			knowledgebootstrap.NewIngestionTaskObserver(knowledgeRuntime.DocumentService),
		))
	}

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

	userRuntime, err := userbootstrap.NewRuntime(context.Background(), userbootstrap.RuntimeOptions{
		Config: cfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init user runtime failed: %v\n", err)
		os.Exit(1)
	}

	r := gin.New()
	r.Use(umw.RequestIDMiddleware())
	r.Use(umw.LogContextMiddleware())
	r.Use(umw.ErrorHandlerMiddleware())
	loginIDExtractor := umw.DefaultLoginIDExtractor
	if cfg != nil && cfg.App.DemoMode {
		loginIDExtractor = umw.DefaultLoginIDExtractorWithDemo
	}
	r.Use(umw.UserContextMiddleware(userRuntime.LoadLoginUser, loginIDExtractor))

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	registerDebugAIRoutes(r, aiRuntime)
	registerKnowledgeRoutes(r, cfg, knowledgeRuntime)
	registerIngestionRoutes(r, cfg, ingestionRuntime)
	registerUserRoutes(r, cfg, userRuntime)
	registerRagRoutes(r, cfg, ragRuntime)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	// 启动 HTTP server
	go func() {
		fmt.Printf("starting server at %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("shutting down server...")

	// 关闭 HTTP server，停止接收新请求
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "server forced to shutdown: %v\n", err)
	}

	// 按逆序关闭各 runtime
	closeRuntime("user", userRuntime.Close)
	closeRuntime("rag", ragRuntime.Close)
	closeRuntime("ingestion", ingestionRuntime.Close)
	closeRuntime("knowledge", knowledgeRuntime.Close)

	fmt.Println("server exited")
}

// closeRuntime 安全关闭 runtime，记录错误但不中断其他 runtime 的关闭。
func closeRuntime(name string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := closeFn(); err != nil {
		fmt.Fprintf(os.Stderr, "close %s runtime failed: %v\n", name, err)
	}
}

// resolveContextPath 从配置中解析 context path，返回对应路由组。
func resolveContextPath(r *gin.Engine, cfg *config.Config) gin.IRouter {
	if cfg == nil {
		return r
	}
	contextPath := strings.Trim(strings.TrimSpace(cfg.Server.Servlet.ContextPath), "/")
	if contextPath == "" {
		return r
	}
	return r.Group("/" + contextPath)
}

func registerKnowledgeRoutes(r *gin.Engine, cfg *config.Config, runtime *knowledgebootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	admin := resolveContextPath(r, cfg).Group("/")
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
	admin := resolveContextPath(r, cfg).Group("/")
	admin.Use(umw.RequireLogin(), umw.RequireRole("admin"))
	ingestionhttp.RegisterPipelineRoutes(admin, runtime.Pipeline)
	ingestionhttp.RegisterTaskRoutes(admin, runtime.Task)
	ingestionhttp.RegisterMetricsRoutes(admin, runtime.Metrics)
}

func registerUserRoutes(r *gin.Engine, cfg *config.Config, runtime *userbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	userhttp.RegisterUserRoutes(resolveContextPath(r, cfg), runtime.AuthService, runtime.UserService)
}

func registerRagRoutes(r *gin.Engine, cfg *config.Config, runtime *ragbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	protected := resolveContextPath(r, cfg).Group("/")
	protected.Use(umw.RequireLogin())
	raghttp.RegisterRoutes(protected, runtime.Conversation, runtime.Message, runtime.Memory, runtime.Feedback, runtime.Chat, runtime.Trace, runtime.CacheMetrics)
}
