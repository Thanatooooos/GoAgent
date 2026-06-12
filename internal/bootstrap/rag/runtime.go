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
	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	raghistory "local/rag-project/internal/app/rag/core/history"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/app/rag/port"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
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
	summaryJobWorker            *raghistory.InMemorySummaryJobWorker
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

	buildCtx, err := newBuildContext(options)
	if err != nil {
		return nil, err
	}

	repos := buildRepositories(buildCtx.db)
	conversation := buildConversationServices(buildCtx, repos)
	memory := buildMemoryServices(buildCtx, repos)
	retrieve := buildRetrieveServices(buildCtx, repos, memory)
	chat, err := buildChatService(buildCtx, repos, conversation, memory, retrieve)
	if err != nil {
		if buildCtx.ownsDB {
			_ = closeRuntimeDB(buildCtx.db)
		}
		return nil, err
	}

	runtime := &Runtime{
		DB:               buildCtx.db,
		ownsDB:           buildCtx.ownsDB,
		mcpManager:       chat.mcpManager,
		memoryCache:      memory.memoryCacheClient,
		summaryJobWorker: conversation.summaryJobWorker,
		CacheMetrics:     memory.memoryCacheMetrics,
		Retrieve:         retrieve.retrieveService,
		Conversation:     conversation.conversationService,
		Message:          conversation.messageService,
		Memory:           memory.explicitMemoryService,
		Feedback:         conversation.feedbackService,
		Trace:            retrieve.traceService,
		Chat:             chat.chatService,
	}
	runtime.startMemoryMaintenanceLoop(buildCtx.cfg)
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
	if r.summaryJobWorker != nil {
		r.summaryJobWorker.Stop()
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

func buildTermNormalizationRules(rules []config.RagTermNormalizationRule) []ragrewrite.TermNormalizationRule {
	if len(rules) == 0 {
		return nil
	}
	result := make([]ragrewrite.TermNormalizationRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, ragrewrite.TermNormalizationRule{
			Canonical: strings.TrimSpace(rule.Canonical),
			Aliases:   append([]string(nil), rule.Aliases...),
			Category:  strings.TrimSpace(rule.Category),
			Version:   rule.Version,
			Enabled:   rule.Enabled,
		})
	}
	return result
}
