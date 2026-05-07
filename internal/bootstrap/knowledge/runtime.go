package knowledge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	sqlcqueries "local/rag-project/internal/adapter/repository/postgres/sqlc"
	s3storage "local/rag-project/internal/adapter/storage/s3"
	taskrocketmq "local/rag-project/internal/adapter/taskqueue/rocketmq"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/app/knowledge/port"
	knowledgeschedule "local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"
)

type RuntimeOptions struct {
	Config          *config.Config
	AIRuntime       *infraai.Runtime
	Storage         port.FileStorage
	VectorStore     port.VectorStore
	DisableRocketMQ bool
}

type Runtime struct {
	DB      *gorm.DB
	PGXPool *pgxpool.Pool

	BaseService            *service.KnowledgeBaseService
	DocumentService        *service.KnowledgeDocumentService
	ChunkService           *service.KnowledgeChunkService
	DocumentProcessService *service.DocumentProcessService
	ScheduleService        *service.KnowledgeDocumentScheduleService

	Storage     port.FileStorage
	VectorStore port.VectorStore

	TaskQueue             *taskrocketmq.TaskQueue
	ChunkDocumentConsumer *taskrocketmq.ChunkDocumentConsumer
	ScheduleJob           *knowledgeschedule.KnowledgeDocumentScheduleJob

	scheduleLoopCancel context.CancelFunc
	scheduleLoopWG     sync.WaitGroup
}

func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	cfg := options.Config
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil {
		return nil, fmt.Errorf("knowledge config is required")
	}

	db, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
	if err != nil {
		return nil, fmt.Errorf("create knowledge gorm db: %w", err)
	}

	pgxPool, err := postgresrepo.NewPGXPool(ctx, cfg.Spring.Datasource)
	if err != nil {
		closeGormDB(db)
		return nil, fmt.Errorf("create knowledge pgx pool: %w", err)
	}

	runtime := &Runtime{
		DB:      db,
		PGXPool: pgxPool,
	}

	queries := sqlcqueries.New(pgxPool)
	baseRepo := postgresknowledge.NewKnowledgeBaseRepository(db)
	documentRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, queries)
	chunkRepo := postgresknowledge.NewKnowledgeChunkRepository(db)
	chunkLogRepo := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(db)
	scheduleRepo := postgresknowledge.NewKnowledgeDocumentScheduleRepository(db)
	scheduleExecRepo := postgresknowledge.NewKnowledgeDocumentScheduleExecRepository(db)

	runtime.BaseService = service.NewKnowledgeBaseService(baseRepo, documentRepo)
	runtime.ScheduleService = service.NewKnowledgeDocumentScheduleService(
		scheduleRepo,
		scheduleExecRepo,
		int64(cfg.Rag.Knowledge.Schedule.MinIntervalSeconds),
		postgresknowledge.NewKnowledgeDocumentScheduleTransaction(db),
	)

	storage := options.Storage
	if storage == nil {
		createdStorage, err := s3storage.NewFileStorage(cfg.RustFS)
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("create file storage: %w", err)
		}
		storage = createdStorage
	}
	runtime.Storage = storage
	remoteFetcher := knowledgeschedule.NewRemoteFileFetcher(knowledgeschedule.RemoteFileFetcherOptions{
		Storage: storage,
	})

	vectorStore := options.VectorStore
	if vectorStore == nil && normalizeVectorType(cfg.Rag.Vector.Type) == "pg" {
		vectorStore = pgvectorstore.NewVectorStore(db)
	}
	runtime.VectorStore = vectorStore

	taskQueue, err := startTaskQueue(cfg, options.DisableRocketMQ)
	if err != nil {
		_ = runtime.Close()
		return nil, err
	}
	runtime.TaskQueue = taskQueue

	aiRuntime := options.AIRuntime
	if aiRuntime == nil {
		aiRuntime = infraai.NewRuntime()
	}

	runtime.DocumentService = service.NewKnowledgeDocumentService(
		baseRepo,
		documentRepo,
		nil,
		chunkLogRepo,
		nil,
		storage,
		taskQueue,
		runtime.ScheduleService,
		remoteFetcher,
		postgresknowledge.NewKnowledgeDocumentDeleteTransaction(db),
	)
	runtime.ChunkService = service.NewKnowledgeChunkService(
		baseRepo,
		documentRepo,
		chunkRepo,
		vectorStore,
		aiRuntime.Embedding,
		postgresknowledge.NewKnowledgeChunkTransaction(db),
	)

	if storage == nil {
		log.Warnf("knowledge document processor not started: file storage adapter is missing")
		return runtime, nil
	}
	if vectorStore == nil {
		log.Warnf("knowledge document processor not started: vector store adapter is missing")
		return runtime, nil
	}
	if aiRuntime.Embedding == nil {
		log.Warnf("knowledge document processor not started: embedding service is missing")
		return runtime, nil
	}

	runtime.DocumentProcessService = service.NewDocumentProcessService(service.DocumentProcessServiceOptions{
		BaseRepo:     baseRepo,
		DocumentRepo: documentRepo,
		ChunkRepo:    chunkRepo,
		ChunkLogRepo: chunkLogRepo,
		Storage:      storage,
		VectorStore:  vectorStore,
		Transaction:  postgresknowledge.NewDocumentProcessTransaction(db),
		Embedding:    aiRuntime.Embedding,
	})

	runtime.ScheduleJob = knowledgeschedule.NewKnowledgeDocumentScheduleJobWithOptions(
		scheduleRepo,
		*knowledgeschedule.NewDocumentStatusHelper(documentRepo),
		knowledgeschedule.KnowledgeDocumentScheduleJobOptions{
			Processor: knowledgeschedule.NewScheduleRefreshProcessor(knowledgeschedule.ScheduleRefreshProcessorOptions{
				ScheduleRepo:      scheduleRepo,
				DocumentRepo:      documentRepo,
				ExecRepo:          scheduleExecRepo,
				Storage:           storage,
				RemoteFileFetcher: remoteFetcher,
				DocumentProcessor: runtime.DocumentProcessService,
			}),
			BatchSize: cfg.Rag.Knowledge.Schedule.BatchSize,
		},
	)

	runtime.startScheduleLoop(cfg)

	if !options.DisableRocketMQ {
		consumer, err := taskrocketmq.NewChunkDocumentConsumer(
			taskrocketmq.ChunkDocumentConsumerOptionsFromConfig(cfg),
			runtime.DocumentProcessService,
		)
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("create chunk document consumer: %w", err)
		}
		if err := consumer.Start(); err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("start chunk document consumer: %w", err)
		}
		runtime.ChunkDocumentConsumer = consumer
	}
	return runtime, nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	var firstErr error
	if r.scheduleLoopCancel != nil {
		r.scheduleLoopCancel()
		r.scheduleLoopWG.Wait()
	}
	if r.ScheduleJob != nil {
		r.ScheduleJob.Close()
	}
	if r.ChunkDocumentConsumer != nil {
		if err := r.ChunkDocumentConsumer.Shutdown(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.TaskQueue != nil {
		if err := r.TaskQueue.Shutdown(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.PGXPool != nil {
		r.PGXPool.Close()
	}
	if err := closeGormDB(r.DB); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (r *Runtime) startScheduleLoop(cfg *config.Config) {
	if r == nil || r.ScheduleJob == nil {
		return
	}
	delay := scheduleScanInterval(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	r.scheduleLoopCancel = cancel
	r.scheduleLoopWG.Add(1)

	go func() {
		defer r.scheduleLoopWG.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("knowledge schedule loop panic recovered: %v", recovered)
			}
		}()

		ticker := time.NewTicker(delay)
		defer ticker.Stop()

		run := func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Errorf("knowledge schedule loop tick panic recovered: %v", recovered)
				}
			}()
			if err := r.ScheduleJob.RecoverStuckRunningDocuments(ctx); err != nil {
				log.Warnf("knowledge schedule recover stuck running documents failed: %v", err)
			}
			if err := r.ScheduleJob.Scan(ctx); err != nil {
				log.Warnf("knowledge schedule scan failed: %v", err)
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

func startTaskQueue(cfg *config.Config, disabled bool) (*taskrocketmq.TaskQueue, error) {
	if disabled {
		return nil, nil
	}

	taskQueue, err := taskrocketmq.NewTaskQueue(taskrocketmq.TaskQueueOptionsFromConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("create rocketmq task queue: %w", err)
	}
	if err := taskQueue.Start(); err != nil {
		return nil, fmt.Errorf("start rocketmq task queue: %w", err)
	}
	return taskQueue, nil
}

func closeGormDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
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

func scheduleScanInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Rag.Knowledge.Schedule.ScanDelayMs <= 0 {
		return 10 * time.Second
	}
	return time.Duration(cfg.Rag.Knowledge.Schedule.ScanDelayMs) * time.Millisecond
}
