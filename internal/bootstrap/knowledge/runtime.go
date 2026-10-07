package knowledge

import (
	"context"
	"errors"
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
	taskgoroutine "local/rag-project/internal/adapter/taskqueue/goroutine"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/core/vision"
	"local/rag-project/internal/app/knowledge/port"
	knowledgeschedule "local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/app/knowledge/service/imageevidence"
	wikiservice "local/rag-project/internal/app/knowledge/service/wiki"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"
)

type RuntimeOptions struct {
	Config      *config.Config
	AIRuntime   *infraai.Runtime
	Storage     port.FileStorage
	VectorStore port.VectorStore
}

type Runtime struct {
	DB      *gorm.DB
	PGXPool *pgxpool.Pool

	BaseService            *service.KnowledgeBaseService
	DocumentService        *service.KnowledgeDocumentService
	ChunkService           *service.KnowledgeChunkService
	WikiPageService        *wikiservice.WikiPageService
	DocumentProcessService *service.DocumentProcessService
	ImageEvidenceService   *imageevidence.Service
	ScheduleService        *service.KnowledgeDocumentScheduleService

	Storage     port.FileStorage
	VectorStore port.VectorStore

	TaskQueue   port.TaskQueue
	ScheduleJob *knowledgeschedule.KnowledgeDocumentScheduleJob

	scheduleLoopCancel context.CancelFunc
	scheduleLoopWG     sync.WaitGroup
	imageWorkerCancel  context.CancelFunc
	imageService       *imageevidence.Service
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
	wikiPageRepo := postgresknowledge.NewWikiPageRepository(db)
	wikiLinkRepo := postgresknowledge.NewWikiLinkRepository(db)

	runtime.WikiPageService = wikiservice.NewWikiPageService(wikiPageRepo, wikiLinkRepo)

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

	aiRuntime := options.AIRuntime
	if aiRuntime == nil {
		aiRuntime = infraai.NewRuntime()
	}

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

	var ocr coreparser.OCRClient
	if strings.TrimSpace(cfg.Parser.OCR.URL) != "" {
		ocr = coreparser.NewHTTPOCRClient(cfg.Parser.OCR.URL, time.Duration(cfg.Parser.OCR.TimeoutMs)*time.Millisecond)
	}
	visionKey := strings.TrimSpace(cfg.Parser.Vision.APIKey)
	if visionKey == "" {
		visionKey = strings.TrimSpace(cfg.AI.Providers["siliconflow"].ApiKey)
	}
	parserSelector := coreparser.NewDefaultSelector(nil)
	imageService := &imageevidence.Service{
		DB: db, Storage: storage, OCR: ocr,
		Vision: vision.NewClient(cfg.Parser.Vision.URL, visionKey,
			time.Duration(cfg.Parser.Vision.TimeoutMs)*time.Millisecond, cfg.Parser.Vision.MaxTokens),
		Embedding:      aiRuntime.Embedding,
		SummaryChat:    aiRuntime.Chat,
		SummaryEnabled: cfg.Rag.Knowledge.Enrichment.Enabled,
	}
	if selected, ok := parserSelector.Select(coreparser.ParserTypeDocReader); ok {
		imageService.InventoryParser, _ = selected.(*coreparser.DocReaderDocumentParser)
	}
	runtime.imageService = imageService
	runtime.ImageEvidenceService = imageService
	runtime.DocumentProcessService = service.NewDocumentProcessService(service.DocumentProcessServiceOptions{
		Jobs:              postgresknowledge.NewChunkJobs(db),
		BaseRepo:          baseRepo,
		DocumentRepo:      documentRepo,
		ChunkRepo:         chunkRepo,
		ChunkLogRepo:      chunkLogRepo,
		Storage:           storage,
		VectorStore:       vectorStore,
		Transaction:       postgresknowledge.NewDocumentProcessTransaction(db),
		Embedding:         aiRuntime.Embedding,
		Chat:              aiRuntime.Chat,
		EnrichmentEnabled: &cfg.Rag.Knowledge.Enrichment.Enabled,
		ImageEvidence:     imageService,
		Parser:            parserSelector,
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

	runtime.TaskQueue = taskgoroutine.NewDurableTaskQueue(runtime.DocumentProcessService, postgresknowledge.NewChunkJobs(db), 5)
	imageWorkerCtx, imageWorkerCancel := context.WithCancel(context.Background())
	runtime.imageWorkerCancel = imageWorkerCancel
	imageService.Run(imageWorkerCtx, 2)

	runtime.DocumentService = service.NewKnowledgeDocumentService(
		baseRepo,
		documentRepo,
		nil,
		chunkLogRepo,
		nil,
		storage,
		runtime.TaskQueue,
		runtime.ScheduleService,
		remoteFetcher,
		postgresknowledge.NewKnowledgeDocumentDeleteTransaction(db),
	)
	runtime.DocumentService.SetChunkJobs(postgresknowledge.NewChunkJobs(db))

	runtime.startScheduleLoop(cfg)

	return runtime, nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	var firstErr error
	if r.imageWorkerCancel != nil {
		r.imageWorkerCancel()
		if r.imageService != nil {
			r.imageService.Wait()
		}
	}
	if r.scheduleLoopCancel != nil {
		r.scheduleLoopCancel()
		r.scheduleLoopWG.Wait()
	}
	if r.ScheduleJob != nil {
		r.ScheduleJob.Close()
	}
	if r.TaskQueue != nil {
		if q, ok := r.TaskQueue.(*taskgoroutine.TaskQueue); ok {
			q.Shutdown()
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
			runCtx, runCancel := context.WithTimeout(ctx, scheduleRunTimeout(cfg))
			defer runCancel()
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Errorf("knowledge schedule loop tick panic recovered: %v", recovered)
				}
			}()
			if err := r.ScheduleJob.RecoverStuckRunningDocuments(runCtx); err != nil {
				log.Warnf("knowledge schedule recover stuck running documents failed: %v", err)
			}
			if err := r.ScheduleJob.Scan(runCtx); err != nil {
				log.Warnf("knowledge schedule scan failed: %v", err)
			}
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				log.Warnf("knowledge schedule loop iteration timed out after %s", scheduleRunTimeout(cfg))
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

func scheduleRunTimeout(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Rag.Knowledge.Schedule.RunTimeoutMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(cfg.Rag.Knowledge.Schedule.RunTimeoutMs) * time.Millisecond
}
