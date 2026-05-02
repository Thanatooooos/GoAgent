package ingestion

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresingestion "local/rag-project/internal/adapter/repository/postgres/ingestion"
	ingestionmodels "local/rag-project/internal/adapter/repository/postgres/ingestion/models"
	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	"local/rag-project/internal/framework/config"
)

// Runtime 聚合 ingestion 模块当前骨架提供的服务。
type Runtime struct {
	DB       *gorm.DB
	ownsDB   bool
	Pipeline *ingestionservice.PipelineService
	Task     *ingestionservice.TaskService
	Executor *ingestionservice.ExecutorService
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
	taskObserver := ingestionservice.NewRepositoryTaskObserver(taskRepo, taskNodeRepo)
	nodeRunners := ingestionservice.NewNodeRunnerRegistry(
		ingestionservice.NewFetcherNodeRunner(),
		ingestionservice.NewParserNodeRunner(coreparser.NewDefaultSelector(nil)),
		ingestionservice.NewChunkerNodeRunner(corechunk.NewDefaultSelector()),
		ingestionservice.NewIndexerNodeRunner(),
	)
	executor := ingestionservice.NewExecutorService(ingestionservice.ExecutorServiceOptions{
		TaskRepo:        taskRepo,
		TaskNodeRepo:    taskNodeRepo,
		WorkflowBuilder: ingestionservice.NewLinearWorkflowBuilder(),
		NodeRunners:     nodeRunners,
		TaskObserver:    taskObserver,
	})

	return &Runtime{
		DB:       db,
		ownsDB:   ownsDB,
		Pipeline: ingestionservice.NewPipelineService(pipelineRepo),
		Task:     ingestionservice.NewTaskService(pipelineRepo, taskRepo, taskNodeRepo, executor),
		Executor: executor,
	}, nil
}

// Close 关闭 ingestion runtime 持有的资源。
func (r *Runtime) Close() error {
	if r == nil || r.DB == nil || !r.ownsDB {
		return nil
	}
	return closeRuntimeDB(r.DB)
}

// ensureIngestionSchema 确保 ingestion 所需表存在。
func ensureIngestionSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("ingestion db is required")
	}
	return db.AutoMigrate(
		&ingestionmodels.PipelineModel{},
		&ingestionmodels.TaskModel{},
		&ingestionmodels.TaskNodeModel{},
	)
}

// closeRuntimeDB 关闭 runtime 内部持有的数据库连接。
func closeRuntimeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
