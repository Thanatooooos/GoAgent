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

	dailybriefhttp "local/rag-project/internal/adapter/http/dailybrief"
	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	raghttp "local/rag-project/internal/adapter/http/rag"
	scheduledtaskhttp "local/rag-project/internal/adapter/http/scheduledtask"
	settingshttp "local/rag-project/internal/adapter/http/settings"
	userhttp "local/rag-project/internal/adapter/http/user"
	workhttp "local/rag-project/internal/adapter/http/work"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	corevector "local/rag-project/internal/app/rag/core/vector"
	conversationruntime "local/rag-project/internal/app/runtime"
	scheduledservice "local/rag-project/internal/app/scheduledtask/service"
	dailybriefbootstrap "local/rag-project/internal/bootstrap/dailybrief"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	scheduledtaskbootstrap "local/rag-project/internal/bootstrap/scheduledtask"
	userbootstrap "local/rag-project/internal/bootstrap/user"
	workbootstrap "local/rag-project/internal/bootstrap/work"
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
	if expected := os.Getenv("APP_EXPECTED_DATABASE"); expected != "" {
		var actual string
		if err := initDB.Raw("SELECT current_database()").Scan(&actual).Error; err != nil || actual != expected {
			fmt.Fprintf(os.Stderr, "database verification failed: expected %q, actual %q\n", expected, actual)
			os.Exit(1)
		}
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

	var ragSearcher corevector.Searcher
	if knowledgeRuntime.VectorStore != nil {
		if searcher, ok := knowledgeRuntime.VectorStore.(corevector.Searcher); ok {
			ragSearcher = searcher
		}
	}
	ragRuntime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{
		Config:                    cfg,
		DB:                        knowledgeRuntime.DB,
		AIRuntime:                 aiRuntime,
		Searcher:                  ragSearcher,
		DisableProfileObservation: os.Getenv("APP_DISABLE_PROFILE_OBSERVATION") == "true",
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

	briefMode := os.Getenv("APP_DAILY_BRIEF_SCHEDULER")
	if briefMode != "" && briefMode != "mixed" && briefMode != "scheduled" {
		fmt.Fprintln(os.Stderr, "APP_DAILY_BRIEF_SCHEDULER must be mixed or scheduled")
		os.Exit(1)
	}
	if briefMode == "scheduled" {
		var remaining int64
		if err := knowledgeRuntime.DB.Raw(`SELECT count(*) FROM t_daily_brief_subscription s WHERE enabled=1 AND NOT EXISTS (SELECT 1 FROM t_daily_brief_task_binding b WHERE b.user_id=s.user_id)`).Scan(&remaining).Error; err != nil || remaining != 0 {
			fmt.Fprintln(os.Stderr, "cannot stop legacy DailyBrief scheduler: enabled subscriptions remain unmigrated or query failed")
			os.Exit(1)
		}
	}
	dailyBriefRuntime, err := dailybriefbootstrap.NewRuntime(context.Background(), dailybriefbootstrap.RuntimeOptions{
		UseScheduledTasks: briefMode == "scheduled",
		DisableSchedule:   os.Getenv("APP_DISABLE_SCHEDULED_JOBS") == "true" || briefMode == "scheduled" || os.Getenv("APP_DISABLE_LEGACY_DAILY_BRIEF") == "true",
		Config:            cfg,
		DB:                knowledgeRuntime.DB,
		TaskRuntime:       ragRuntime.TaskRuntime,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init daily brief runtime failed: %v\n", err)
		os.Exit(1)
	}
	scheduledTaskRuntime, err := scheduledtaskbootstrap.NewRuntime(knowledgeRuntime.DB, ragRuntime.TaskRuntime, cfg.ScheduledTask)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init scheduled task runtime failed: %v\n", err)
		os.Exit(1)
	}
	if os.Getenv("APP_DISABLE_SCHEDULED_JOBS") != "true" {
		scheduledTaskRuntime.Start()
	}
	workRuntime, err := workbootstrap.NewRuntime(knowledgeRuntime.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init work runtime failed: %v\n", err)
		os.Exit(1)
	}
	baseKernel, ok := ragRuntime.ConversationRuntime.(*conversationruntime.Runtime)
	workRuntime.ConfigureMaterials(knowledgeRuntime, cfg.AI.Embedding.DefaultModel)
	workRuntime.StartCleanup()
	if !ok {
		fmt.Fprintln(os.Stderr, "work requires the conversation runtime kernel")
		os.Exit(1)
	}
	if err := workRuntime.ConfigureChat(baseKernel, runtimeadapter.NewConversations(ragRuntime.Conversation), runtimeadapter.NewConversationMessages(ragRuntime.Message, postgresrag.NewConversationMessageChunkSink(ragRuntime.DB, ragRuntime.Embedding)), ragRuntime.StreamManager); err != nil {
		fmt.Fprintf(os.Stderr, "init work chat failed: %v\n", err)
		os.Exit(1)
	}

	r := gin.New()
	loginIDExtractor := umw.DefaultLoginIDExtractor
	if cfg != nil && cfg.App.DemoMode {
		loginIDExtractor = umw.DefaultLoginIDExtractorWithDemo
	}
	registerCoreMiddleware(r, userRuntime.LoadLoginUser, loginIDExtractor)
	r.Use(umw.RejectWorkPrivateKnowledge(knowledgeRuntime.DB))

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	registerDebugAIRoutes(r, aiRuntime)
	registerKnowledgeRoutes(r, cfg, knowledgeRuntime)
	registerUserRoutes(r, cfg, userRuntime)
	registerRagRoutes(r, cfg, ragRuntime)
	registerDailyBriefRoutes(r, cfg, dailyBriefRuntime)
	registerScheduledTaskRoutes(r, cfg, scheduledTaskRuntime, ragRuntime)
	registerWorkRoutes(r, cfg, workRuntime)

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
	closeRuntime("daily-brief", dailyBriefRuntime.Close)
	closeRuntime("work", workRuntime.Close)
	closeRuntime("scheduled-task", scheduledTaskRuntime.Close)
	closeRuntime("user", userRuntime.Close)
	closeRuntime("rag", ragRuntime.Close)
	closeRuntime("knowledge", knowledgeRuntime.Close)

	fmt.Println("server exited")
}

func registerCoreMiddleware(r *gin.Engine, loader umw.UserLoaderFunc, extractor umw.LoginIDExtractor) {
	if r == nil {
		return
	}
	r.Use(umw.RequestIDMiddleware())
	r.Use(umw.LogContextMiddleware())
	r.Use(umw.AccessLogMiddleware())
	r.Use(umw.ErrorHandlerMiddleware())
	r.Use(umw.UserContextMiddleware(loader, extractor))
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
	knowledgehttp.RegisterImageEvidenceAdminRoutes(admin, runtime.ImageEvidenceService)
	protected := resolveContextPath(r, cfg).Group("/")
	protected.Use(umw.RequireLogin())
	knowledgehttp.RegisterImageEvidenceReadRoutes(protected, runtime.ImageEvidenceService)
	knowledgehttp.RegisterWikiPageRoutes(admin, runtime.WikiPageService)
	settingshttp.RegisterRoutes(admin, cfg)
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
	raghttp.RegisterRoutes(protected, runtime.Conversation, runtime.Message, runtime.Memory, runtime.Feedback, runtime.PreferenceCandidates, runtime.Trace, runtime.CacheMetrics, runtime.StreamManager, runtime.RuntimeChat)
}

func registerDailyBriefRoutes(r *gin.Engine, cfg *config.Config, runtime *dailybriefbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	protected := resolveContextPath(r, cfg).Group("/")
	protected.Use(umw.RequireLogin())
	dailybriefhttp.RegisterRoutes(protected, runtime.ReadService, runtime.SubscriptionService, runtime.TopicCatalogService)

	admin := resolveContextPath(r, cfg).Group("/")
	admin.Use(umw.RequireLogin(), umw.RequireRole("admin"))
	dailybriefhttp.RegisterAdminRoutes(admin, runtime.SubscriptionSnapshotService)
}

func registerScheduledTaskRoutes(r *gin.Engine, cfg *config.Config, runtime *scheduledtaskbootstrap.Runtime, ragRuntime *ragbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	protected := resolveContextPath(r, cfg).Group("/")
	protected.Use(umw.RequireLogin())
	scheduledtaskhttp.RegisterRoutes(protected, runtime.Store, scheduledservice.Proposer{Model: ragRuntime.LLMChat})
}

func registerWorkRoutes(r *gin.Engine, cfg *config.Config, runtime *workbootstrap.Runtime) {
	if r == nil || runtime == nil {
		return
	}
	workhttp.RegisterRoutes(resolveContextPath(r, cfg), runtime.Service)
	protected := resolveContextPath(r, cfg).Group("/")
	protected.Use(umw.RequireLogin())
	workhttp.RegisterChatRoutes(protected, runtime)
	workhttp.RegisterProposalRoutes(protected, runtime)
	workhttp.RegisterMaterialRoutes(protected, runtime)
}
