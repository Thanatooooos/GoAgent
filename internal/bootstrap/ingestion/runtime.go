package ingestion

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/feishu"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresingestion "local/rag-project/internal/adapter/repository/postgres/ingestion"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	s3storage "local/rag-project/internal/adapter/storage/s3"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
)

// Runtime 聚合 ingestion 模块当前骨架提供的服务。
type Runtime struct {
	DB       *gorm.DB
	ownsDB   bool
	Pipeline *ingestionservice.PipelineService
	Task     *ingestionservice.TaskService
	Executor *ingestionservice.ExecutorService
	Metrics  *ingestionservice.MetricsService
}

// RuntimeOptions 描述 ingestion runtime 的装配参数。
type RuntimeOptions struct {
	Config *config.Config
	DB     *gorm.DB
}

// NewRuntime 创建 ingestion runtime 骨架。
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	_ = ctx

	cfg := options.Config
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil && options.DB == nil {
		return nil, fmt.Errorf("ingestion config or db is required")
	}

	db := options.DB
	ownsDB := false
	if db == nil {
		createdDB, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
		if err != nil {
			return nil, fmt.Errorf("create ingestion gorm db: %w", err)
		}
		db = createdDB
		ownsDB = true
	}
	if err := ensureIngestionSchema(db); err != nil {
		if ownsDB {
			_ = closeRuntimeDB(db)
		}
		return nil, fmt.Errorf("ensure ingestion schema: %w", err)
	}

	pipelineRepo := postgresingestion.NewPipelineRepository(db)
	taskRepo := postgresingestion.NewTaskRepository(db)
	taskNodeRepo := postgresingestion.NewTaskNodeRepository(db)
	baseRepo := postgresknowledge.NewKnowledgeBaseRepository(db)
	chunkRepo := postgresknowledge.NewKnowledgeChunkRepository(db)
	metricsService := ingestionservice.NewMetricsService(readIngestionMaxConcurrent(cfg))
	taskObserver := ingestionservice.NewMultiTaskObserver(
		ingestionservice.NewRepositoryTaskObserver(taskRepo, taskNodeRepo),
		ingestionservice.NewMetricsObserver(metricsService),
	)

	var storage knowledgeport.FileStorage
	if cfg != nil {
		storageAdapter, err := s3storage.NewFileStorage(cfg.RustFS)
		if err == nil {
			storage = storageAdapter
		}
	}
	var vectorStore knowledgeport.VectorStore
	if cfg == nil || normalizeVectorType(cfg.Rag.Vector.Type) == "pg" {
		vectorStore = pgvectorstore.NewVectorStore(db)
	}
	aiRuntime := infraai.NewRuntime()

	// 装配 fetcher 并注入飞书客户端。
	fetcher := ingestionservice.NewFetcherNodeRunner(storage, &http.Client{})
	if cfg != nil && cfg.Feishu.AppID != "" && cfg.Feishu.AppSecret != "" {
		fetcher.SetFeishuClient(feishu.NewClient(cfg.Feishu.AppID, cfg.Feishu.AppSecret))
	}

	nodeRunners := ingestionservice.NewNodeRunnerRegistry(
		fetcher,
		ingestionservice.NewParserNodeRunner(coreparser.NewDefaultSelector(nil)),
		ingestionservice.NewEnhancerNodeRunner(),
		ingestionservice.NewChunkerNodeRunner(corechunk.NewDefaultSelector()),
		ingestionservice.NewEnricherNodeRunner(),
		ingestionservice.NewIndexerNodeRunner(baseRepo, chunkRepo, vectorStore, aiRuntime.Embedding),
	)
	executor := ingestionservice.NewExecutorService(ingestionservice.ExecutorServiceOptions{
		TaskRepo:        taskRepo,
		TaskNodeRepo:    taskNodeRepo,
		WorkflowBuilder: ingestionservice.NewEinoGraphWorkflowBuilder(),
		NodeRunners:     nodeRunners,
		TaskObserver:    taskObserver,
		Metrics:         metricsService,
		MaxConcurrent:   readIngestionMaxConcurrent(cfg),
		MaxRetries:      readIngestionMaxRetries(cfg),
		RetryBackoffMs:  readIngestionRetryBackoffMs(cfg),
	})
	metricsService.SetMaxConcurrent(executor.MaxConcurrent())

	return &Runtime{
		DB:       db,
		ownsDB:   ownsDB,
		Pipeline: ingestionservice.NewPipelineService(pipelineRepo, nodeRunners),
		Task:     ingestionservice.NewTaskService(pipelineRepo, taskRepo, taskNodeRepo, executor),
		Executor: executor,
		Metrics:  metricsService,
	}, nil
}

// Close 关闭 ingestion runtime 持有的资源。
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	if r.Executor != nil {
		r.Executor.Close()
	}
	if r.DB == nil || !r.ownsDB {
		return nil
	}
	return closeRuntimeDB(r.DB)
}

// ingestionRequiredTables ingestion 模块依赖的数据表。
var ingestionRequiredTables = []string{
	"t_ingestion_pipeline",
	"t_ingestion_task",
	"t_ingestion_task_node",
}

// ensureIngestionSchema 确保 ingestion 依赖的表已通过 migration 创建，不再使用 AutoMigrate。
func ensureIngestionSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("ingestion db is required")
	}
	return postgresrepo.EnsureTablesExist(db, ingestionRequiredTables)
}

// closeRuntimeDB 关闭 runtime 内部持有的数据库连接。
func closeRuntimeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func normalizeVectorType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "pg"
	}
	return value
}

func readIngestionMaxConcurrent(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	return cfg.Rag.Knowledge.Ingestion.MaxConcurrent
}

func readIngestionMaxRetries(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	return cfg.Rag.Knowledge.Ingestion.MaxRetries
}

func readIngestionRetryBackoffMs(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	ms := cfg.Rag.Knowledge.Ingestion.RetryBackoffMs
	if ms <= 0 {
		ms = 1000
	}
	return ms
}
