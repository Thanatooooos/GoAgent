package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	rediscache "local/rag-project/internal/adapter/cache/redis"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresuser "local/rag-project/internal/adapter/repository/postgres/user"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	raghistory "local/rag-project/internal/app/rag/core/history"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/app/rag/port"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragassembly "local/rag-project/internal/app/rag/tool/assembly"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"
	inframcp "local/rag-project/internal/infra-mcp"
)

// RuntimeOptions 描述 RAG runtime 的装配选项。
type RuntimeOptions struct {
	Config    *config.Config
	DB        *gorm.DB
	AIRuntime *infraai.Runtime
	Searcher  corevector.Searcher
}

// Runtime 聚合最小 RAG 闭环需要的服务。
type Runtime struct {
	DB                          *gorm.DB
	ownsDB                      bool
	mcpManager                  *inframcp.Manager
	memoryCache                 *goredis.Client
	memoryMaintenanceLoopCancel context.CancelFunc
	memoryMaintenanceLoopWG     sync.WaitGroup
	memoryMaintenanceRunner     func(context.Context, longtermmemory.MaintenanceInput) (longtermmemory.MaintenanceResult, error)
	CacheMetrics                *ragcachemetrics.Service
	Retrieve                    ragretrieve.Service
	Conversation                *ragservice.ConversationService
	Message                     *ragservice.ConversationMessageService
	Memory                      *longtermmemory.MemoryService
	Feedback                    *ragservice.MessageFeedbackService
	Trace                       *ragservice.TraceService
	Chat                        *ragservice.RagChatService
}

// NewRuntime 创建 RAG 最小运行时。
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	_ = ctx

	cfg := options.Config
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil {
		return nil, fmt.Errorf("rag config is required")
	}

	db := options.DB
	ownsDB := false
	if db == nil {
		createdDB, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
		if err != nil {
			return nil, fmt.Errorf("create rag gorm db: %w", err)
		}
		db = createdDB
		ownsDB = true
	}
	if err := ensureRagSchema(db); err != nil {
		if ownsDB {
			_ = closeRuntimeDB(db)
		}
		return nil, fmt.Errorf("ensure rag schema: %w", err)
	}

	aiRuntime := options.AIRuntime
	if aiRuntime == nil {
		aiRuntime = infraai.NewRuntime()
	}

	searcher := options.Searcher
	if searcher == nil {
		searcher = pgvectorstore.NewVectorStore(db)
	}

	conversationRepo := postgresrag.NewConversationRepository(db)
	messageRepo := postgresrag.NewConversationMessageRepository(db)
	summaryRepo := postgresrag.NewConversationSummaryRepository(db)
	feedbackRepo := postgresrag.NewMessageFeedbackRepository(db)
	memoryItemRepo := postgresrag.NewMemoryItemRepository(db)
	memoryItemEmbeddingRepo := postgresrag.NewMemoryItemEmbeddingRepository(db)
	sessionChunkRepo := postgresrag.NewSessionChunkRepository(db)
	traceRunRepo := postgresrag.NewRagTraceRunRepository(db)
	traceNodeRepo := postgresrag.NewRagTraceNodeRepository(db)
	userRepo := postgresuser.NewUserRepository(db)

	conversationService := ragservice.NewConversationService(
		conversationRepo,
		messageRepo,
		summaryRepo,
		nil,
		nil,
		0,
		postgresrag.NewConversationDeleteTransaction(db),
	)
	messageService := ragservice.NewConversationMessageService(
		conversationRepo,
		messageRepo,
		summaryRepo,
		feedbackRepo,
	)
	messageService.SetContentProcessor(ragservice.NewLongMessageContentProcessor(ragservice.LongMessageProcessorOptions{
		Enabled:                     cfg.Rag.Memory.LongMessage.Enabled,
		DirectContextMaxTokens:      cfg.Rag.Memory.LongMessage.DirectContextMaxTokens,
		ChunkSummaryThresholdTokens: cfg.Rag.Memory.LongMessage.ChunkSummaryThresholdTokens,
		LargeChunkTargetTokens:      cfg.Rag.Memory.LongMessage.LargeChunkTargetTokens,
		LargeChunkOverlapTokens:     cfg.Rag.Memory.LongMessage.LargeChunkOverlapTokens,
		MediumSummaryMaxChars:       cfg.Rag.Memory.LongMessage.MediumSummaryMaxChars,
		ChunkSummaryMaxChars:        cfg.Rag.Memory.LongMessage.ChunkSummaryMaxChars,
		LargeSummaryMaxChars:        cfg.Rag.Memory.LongMessage.LargeSummaryMaxChars,
		Estimator:                   ragservice.RoughTokenEstimator{},
		ChatService:                 aiRuntime.Chat,
	}))
	messageService.SetChunkSink(postgresrag.NewConversationMessageChunkSink(db, aiRuntime.Embedding))
	messageService.SetCreateTransaction(postgresrag.NewConversationMessageCreateTransaction(db, aiRuntime.Embedding))

	memoryStore := raghistory.NewMessageServiceStore(conversationRepo, messageRepo)
	var summaryAdapter raghistory.SummaryService
	if cfg.Rag.Memory.SummaryEnabled {
		summaryAdapter = raghistory.NewCompressibleSummaryService(summaryRepo, raghistory.SummaryCompressionOptions{
			MessageRepo: messageRepo,
			ChatService: aiRuntime.Chat,
			StartTurns:  cfg.Rag.Memory.SummaryStartTurns,
			MaxChars:    cfg.Rag.Memory.SummaryMaxChars,
		})
	} else {
		summaryAdapter = raghistory.NewSummaryServiceAdapter(summaryRepo)
	}
	historyService := raghistory.NewDefaultService(memoryStore, summaryAdapter, cfg.Rag.Memory.HistoryKeepTurns)
	explicitMemoryService := longtermmemory.NewMemoryService(memoryItemRepo, longtermmemory.MemoryServiceOptions{
		MaxRecallItems:        cfg.Rag.Memory.ExplicitRecall.MaxItems,
		MaxRecallChars:        cfg.Rag.Memory.ExplicitRecall.MaxContextChars,
		MaxCandidatesPerScope: cfg.Rag.Memory.ExplicitRecall.MaxCandidatesPerScope,
	})
	explicitMemoryService.SetMutationTransaction(postgresrag.NewMemoryItemTransaction(db))
	explicitMemoryService.SetEmbeddingSupport(aiRuntime.Embedding, memoryItemEmbeddingRepo)
	memoryCacheMetrics := buildMemoryCacheMetrics(cfg)
	explicitMemoryService.SetCacheMetrics(memoryCacheMetrics)
	memoryCacheClient, recallCache := buildMemoryRecallCache(cfg)
	explicitMemoryService.SetRecallCache(recallCache, longtermmemory.RecallCacheOptions{
		Enabled:             cfg.Rag.Memory.Cache.Enabled,
		RequestScopeEnabled: readRequestScopeCacheEnabled(cfg),
		EmbeddingTTL:        time.Duration(cfg.Rag.Memory.Cache.EmbeddingTTLSeconds) * time.Second,
		RuleTTL:             time.Duration(cfg.Rag.Memory.Cache.RuleTTLSeconds) * time.Second,
		FactTTL:             time.Duration(cfg.Rag.Memory.Cache.FactTTLSeconds) * time.Second,
		EmptyFactTTL:        time.Duration(cfg.Rag.Memory.Cache.EmptyFactTTLSeconds) * time.Second,
		EmbeddingModel:      strings.TrimSpace(cfg.AI.Embedding.DefaultModel),
		RankVersion:         defaultMemoryFactRankVersion,
	})

	// 根据配置决定是否启用 LLM 查询改写；未启用时 retieve 阶段直接使用原始问题。
	var rewriteService ragrewrite.Service
	if cfg.Rag.QueryRewrite.Enabled {
		rewriteService = ragrewrite.NewLLMService(aiRuntime.Chat)
	}
	promptService := ragprompt.NewService(nil)
	retrieveService := ragretrieve.NewEngine(searcher, aiRuntime.Embedding, aiRuntime.Rerank)
	retrieveService.SetFactMemoryRetriever(explicitMemoryService.FactRetriever())
	feedbackService := ragservice.NewMessageFeedbackService(messageRepo, feedbackRepo)
	traceService := ragservice.NewTraceService(traceRunRepo, traceNodeRepo, userRepo)
	tracer := ragservice.NewChatTracer(traceRunRepo, traceNodeRepo)
	mcpManager := buildMCPManager(cfg)
	chatService := ragservice.NewRagChatService(
		conversationService,
		messageService,
		historyService,
		rewriteService,
		retrieveService,
		promptService,
		aiRuntime.Chat,
		tracer,
	)
	// 检索置信度阈值：低于此值时回退到通用 LLM 模式并提醒用户。
	if cfg != nil {
		chatService.SetConfidenceThreshold(cfg.Rag.Search.Channels.VectorGlobal.ConfidenceThreshold)
	}
	chatService.SetParallelSubquestionRetrieval(
		cfg.Rag.Retrieve.ParallelSubquestions.Enabled,
		cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency,
	)
	chatService.SetAgentRuntimeMode(cfg.Rag.Agent.Chat.Mode)
	chatService.SetLongTermMemoryRecallService(explicitMemoryService.RecallService())
	sessionRecallService := ragservice.NewSessionRecallService(sessionChunkRepo, aiRuntime.Embedding, ragservice.SessionRecallOptions{
		Enabled:              cfg.Rag.Memory.SessionRecall.Enabled,
		MaxExcerpts:          cfg.Rag.Memory.SessionRecall.MaxExcerpts,
		MaxChunksPerMessage:  cfg.Rag.Memory.SessionRecall.MaxChunksPerMessage,
		ExcerptTargetTokens:  cfg.Rag.Memory.SessionRecall.ExcerptTargetTokens,
		ExcerptOverlapTokens: cfg.Rag.Memory.SessionRecall.ExcerptOverlapTokens,
		MaxPromptTokens:      cfg.Rag.Memory.SessionRecall.MaxPromptTokens,
		Estimator:            ragservice.RoughTokenEstimator{},
	})
	if cacheAware, ok := sessionRecallService.(interface {
		SetCacheSupport(cache port.MemoryRecallCache, options ragservice.SessionRecallCacheOptions)
	}); ok {
		cacheAware.SetCacheSupport(recallCache, ragservice.SessionRecallCacheOptions{
			Enabled:                  cfg.Rag.Memory.Cache.Enabled && readSessionRecallCacheEnabled(cfg),
			RequestScopeEnabled:      readRequestScopeCacheEnabled(cfg),
			ConversationScopeEnabled: readConversationScopeCacheEnabled(cfg),
			ConversationMaxEntries:   cfg.Rag.Memory.Cache.ConversationMaxEntries,
			ConversationTTL:          time.Duration(cfg.Rag.Memory.Cache.ConversationTTLSeconds) * time.Second,
			EmptyResultTTL:           time.Duration(cfg.Rag.Memory.Cache.EmptySessionTTLSeconds) * time.Second,
			EmbeddingTTL:             time.Duration(cfg.Rag.Memory.Cache.EmbeddingTTLSeconds) * time.Second,
			EmbeddingModel:           strings.TrimSpace(cfg.AI.Embedding.DefaultModel),
		})
	}
	if metricAware, ok := sessionRecallService.(interface {
		SetCacheMetrics(metrics *ragcachemetrics.Service)
	}); ok {
		metricAware.SetCacheMetrics(memoryCacheMetrics)
	}
	chatService.SetSessionRecallService(sessionRecallService)
	chatService.SetRequestCacheMaxEntries(readRequestCacheMaxEntries(cfg))
	chatService.SetToolWorkflow(ragassembly.BuildLocalWorkflow(db, traceRunRepo, traceNodeRepo, cfg, mcpManager, aiRuntime.Chat))
	agentRuntimeService, err := buildAgentRuntimeService(cfg, mcpManager, aiRuntime.Chat)
	if err != nil {
		if ownsDB {
			_ = closeRuntimeDB(db)
		}
		return nil, fmt.Errorf("build agent runtime service: %w", err)
	}
	chatService.SetAgentRuntimeService(agentRuntimeService)

	runtime := &Runtime{
		DB:           db,
		ownsDB:       ownsDB,
		mcpManager:   mcpManager,
		memoryCache:  memoryCacheClient,
		CacheMetrics: memoryCacheMetrics,
		Retrieve:     retrieveService,
		Conversation: conversationService,
		Message:      messageService,
		Memory:       explicitMemoryService,
		Feedback:     feedbackService,
		Trace:        traceService,
		Chat:         chatService,
	}
	runtime.startMemoryMaintenanceLoop(cfg)
	return runtime, nil
}

// Close 关闭 runtime 持有的数据库资源。
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var err error
	if r.mcpManager != nil {
		err = errors.Join(err, r.mcpManager.Close())
	}
	if r.memoryCache != nil {
		err = errors.Join(err, r.memoryCache.Close())
	}
	r.stopMemoryMaintenanceLoop()
	if r.DB == nil || !r.ownsDB {
		return err
	}
	return errors.Join(err, closeRuntimeDB(r.DB))
}

// ragRequiredTables RAG 模块依赖的数据表。
var ragRequiredTables = []string{
	"t_conversation",
	"t_conversation_summary",
	"t_message",
	"t_memory_item",
	"t_memory_item_embedding",
	"t_session_chunk",
	"t_session_chunk_embedding",
	"t_message_feedback",
	"t_rag_trace_run",
	"t_rag_trace_node",
}

// ensureRagSchema 确保 RAG 依赖的表已通过 migration 创建，不再使用 AutoMigrate。
func ensureRagSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("rag db is required")
	}
	return postgresrepo.EnsureTablesExist(db, ragRequiredTables)
}

// closeRuntimeDB 关闭 runtime 内部持有的数据库连接。
func closeRuntimeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func buildMCPManager(cfg *config.Config) *inframcp.Manager {
	if cfg == nil {
		return nil
	}
	servers := make(map[string]inframcp.ServerConfig, len(cfg.Rag.MCP.Servers))
	for name, serverCfg := range cfg.Rag.MCP.Servers {
		servers[strings.TrimSpace(name)] = inframcp.ServerConfig{
			Enabled:          serverCfg.Enabled,
			Transport:        serverCfg.Transport,
			Command:          serverCfg.Command,
			Args:             append([]string(nil), serverCfg.Args...),
			Env:              cloneMCPEnv(serverCfg.Env),
			StartupTimeoutMs: serverCfg.StartupTimeoutMs,
			CallTimeoutMs:    serverCfg.CallTimeoutMs,
		}
	}

	serverName := strings.TrimSpace(cfg.Rag.Search.WebSearch.MCP.Server)
	if serverName == "" {
		serverName = "tavily"
	}
	apiKey := strings.TrimSpace(cfg.Rag.Search.WebSearch.ApiKey)
	if apiKey != "" {
		if serverCfg, ok := servers[serverName]; ok {
			if serverCfg.Env == nil {
				serverCfg.Env = map[string]string{}
			}
			if strings.TrimSpace(serverCfg.Env["TAVILY_API_KEY"]) == "" {
				serverCfg.Env["TAVILY_API_KEY"] = apiKey
			}
			servers[serverName] = serverCfg
		}
	}
	return inframcp.NewManager(servers)
}

const defaultMemoryFactRankVersion = "v1"

func buildMemoryRecallCache(cfg *config.Config) (*goredis.Client, port.MemoryRecallCache) {
	if cfg == nil || !cfg.Rag.Memory.Cache.Enabled {
		return nil, nil
	}
	host := strings.TrimSpace(cfg.Spring.Data.Redis.Host)
	port := cfg.Spring.Data.Redis.Port
	if host == "" || port <= 0 {
		return nil, nil
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: cfg.Spring.Data.Redis.Password,
		DB:       cfg.Spring.Data.Redis.DB,
	})
	return client, rediscache.NewRagMemoryCacheWithPrefix(client, cfg.Rag.Memory.Cache.RedisKeyPrefix)
}

func buildMemoryCacheMetrics(cfg *config.Config) *ragcachemetrics.Service {
	if cfg == nil || !readMemoryCacheMetricsEnabled(cfg) {
		return nil
	}
	return ragcachemetrics.NewService()
}

func readRequestScopeCacheEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.Rag.Memory.Cache.Enabled {
		return false
	}
	if cfg.Rag.Memory.Cache.RequestScopeEnabled {
		return true
	}
	return cfg.Rag.Memory.Cache.RequestMaxEntries == 0
}

func readConversationScopeCacheEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.Rag.Memory.Cache.Enabled {
		return false
	}
	if cfg.Rag.Memory.Cache.ConversationScopeEnabled {
		return true
	}
	return cfg.Rag.Memory.Cache.ConversationMaxEntries == 0 && cfg.Rag.Memory.Cache.ConversationTTLSeconds == 0
}

func readSessionRecallCacheEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.Rag.Memory.Cache.Enabled {
		return false
	}
	if cfg.Rag.Memory.Cache.SessionRecallEnabled {
		return true
	}
	return cfg.Rag.Memory.Cache.EmptySessionTTLSeconds == 0
}

func readMemoryCacheMetricsEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.Rag.Memory.Cache.Enabled {
		return false
	}
	return cfg.Rag.Memory.Cache.MetricsEnabled
}

func readRequestCacheMaxEntries(cfg *config.Config) int {
	if cfg == nil || cfg.Rag.Memory.Cache.RequestMaxEntries <= 0 {
		return 128
	}
	return cfg.Rag.Memory.Cache.RequestMaxEntries
}

func (r *Runtime) startMemoryMaintenanceLoop(cfg *config.Config) {
	if r == nil || !readMemoryMaintenanceEnabled(cfg) {
		return
	}
	runner := r.memoryMaintenanceRunner
	if runner == nil {
		if r.Memory == nil {
			return
		}
		runner = r.Memory.RunMaintenance
	}

	input := buildMemoryMaintenanceInput(cfg)
	delay := readMemoryMaintenanceScanDelay(cfg)
	runTimeout := readMemoryMaintenanceRunTimeout(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	r.memoryMaintenanceLoopCancel = cancel
	r.memoryMaintenanceLoopWG.Add(1)

	go func() {
		defer r.memoryMaintenanceLoopWG.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("rag memory maintenance loop panic recovered: %v", recovered)
			}
		}()

		ticker := time.NewTicker(delay)
		defer ticker.Stop()

		run := func() {
			runCtx, runCancel := context.WithTimeout(ctx, runTimeout)
			defer runCancel()
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Errorf("rag memory maintenance tick panic recovered: %v", recovered)
				}
			}()

			result, err := runner(runCtx, input)
			if err != nil {
				log.Warnf("rag memory maintenance failed: %v", err)
			} else if result.ExpiredCount > 0 || result.DeletedCount > 0 {
				log.Infof("rag memory maintenance completed: expired=%d deleted=%d", result.ExpiredCount, result.DeletedCount)
			}
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				log.Warnf("rag memory maintenance iteration timed out after %s", runTimeout)
			}
		}

		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func (r *Runtime) stopMemoryMaintenanceLoop() {
	if r == nil || r.memoryMaintenanceLoopCancel == nil {
		return
	}
	r.memoryMaintenanceLoopCancel()
	r.memoryMaintenanceLoopWG.Wait()
	r.memoryMaintenanceLoopCancel = nil
}

func readMemoryMaintenanceEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.Rag.Memory.Maintenance.Enabled
}

func readMemoryMaintenanceScanDelay(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Rag.Memory.Maintenance.ScanDelayMs <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(cfg.Rag.Memory.Maintenance.ScanDelayMs) * time.Millisecond
}

func readMemoryMaintenanceRunTimeout(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Rag.Memory.Maintenance.RunTimeoutMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(cfg.Rag.Memory.Maintenance.RunTimeoutMs) * time.Millisecond
}

func buildMemoryMaintenanceInput(cfg *config.Config) longtermmemory.MaintenanceInput {
	input := longtermmemory.MaintenanceInput{}
	if cfg == nil {
		return input
	}
	input.ExpireBatchSize = cfg.Rag.Memory.Maintenance.ExpireBatchSize
	input.DeleteBatchSize = cfg.Rag.Memory.Maintenance.DeleteBatchSize
	if days := cfg.Rag.Memory.Maintenance.DeleteRetentionDays; days > 0 {
		input.DeleteRetention = time.Duration(days) * 24 * time.Hour
	}
	return input
}

func cloneMCPEnv(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
