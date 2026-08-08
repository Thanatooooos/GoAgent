package dailybrief

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresdailybrief "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	dailybriefschedule "local/rag-project/internal/app/dailybrief/schedule"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/log"
	infraai "local/rag-project/internal/infra-ai"
)

type Runtime struct {
	DB *gorm.DB

	SubscriptionService         *dailybriefservice.SubscriptionService
	TopicCatalogService         *dailybriefservice.TopicCatalogService
	SubscriptionSnapshotService *dailybriefservice.SubscriptionSnapshotService
	ReadService                 *dailybriefservice.ReadService
	Orchestrator        *dailybriefservice.GenerationOrchestrator
	ScheduleJob         *dailybriefschedule.Job
	Metrics             *dailybriefservice.MetricsService

	ownsDB             bool
	scheduleLoopCancel context.CancelFunc
	scheduleLoopWG     sync.WaitGroup
}

type RuntimeOptions struct {
	Config    *config.Config
	DB        *gorm.DB
	AIRuntime *infraai.Runtime
}

func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	_ = ctx

	cfg := options.Config
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil && options.DB == nil {
		return nil, fmt.Errorf("daily brief config or db is required")
	}

	db := options.DB
	ownsDB := false
	if db == nil {
		createdDB, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
		if err != nil {
			return nil, fmt.Errorf("create daily brief gorm db: %w", err)
		}
		db = createdDB
		ownsDB = true
	}

	runtime := &Runtime{
		DB:     db,
		ownsDB: ownsDB,
	}

	subscriptionRepo := postgresdailybrief.NewSubscriptionRepository(db)
	issueRepo := postgresdailybrief.NewIssueRepository(db)
	itemRepo := postgresdailybrief.NewItemRepository(db)
	generationRunRepo := postgresdailybrief.NewGenerationRunRepository(db)
	publishTx := postgresdailybrief.NewPublishTransaction(db)

	runtime.SubscriptionService = dailybriefservice.NewSubscriptionService(subscriptionRepo)
	runtime.TopicCatalogService = dailybriefservice.NewTopicCatalogService()
	runtime.SubscriptionSnapshotService = dailybriefservice.NewSubscriptionSnapshotService(subscriptionRepo)
	runtime.ReadService = dailybriefservice.NewReadService(subscriptionRepo, issueRepo, itemRepo)
	metrics := dailybriefservice.NewMetricsService()
	runtime.Metrics = metrics

	aiRuntime := options.AIRuntime
	if aiRuntime == nil {
		aiRuntime = infraai.NewRuntime()
	}
	if aiRuntime.Chat == nil {
		log.Warnf("daily brief generation not started: llm service is missing")
		return runtime, nil
	}

	sourceRegistry, err := dailybriefservice.NewDefaultSourceRegistry()
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("create daily brief source registry: %w", err)
	}

	generationCfg := cfg.DailyBrief.Generation
	sourceCollector := dailybriefservice.NewSourceCollector(sourceRegistry, dailybriefservice.NewHTTPClient(nil))
	candidatePipeline := dailybriefservice.NewCandidatePipeline(generationCfg)
	generator := dailybriefservice.NewBriefGenerator(aiRuntime.Chat, generationCfg)
	publisher := dailybriefservice.NewPublisher(publishTx)
	issueService := dailybriefservice.NewIssueService(issueRepo)
	generationRunService := dailybriefservice.NewGenerationRunService(generationRunRepo)

	runtime.Orchestrator = dailybriefservice.NewGenerationOrchestrator(
		sourceCollector,
		candidatePipeline,
		generator,
		publisher,
		issueService,
		generationRunService,
		generationCfg,
		metrics,
	)

	lockManager := dailybriefschedule.NewLockManager(
		subscriptionRepo,
		int64(cfg.DailyBrief.Schedule.LockSeconds),
		time.Now,
	)
	retryPolicy := dailybriefschedule.NewRetryPolicy(
		cfg.DailyBrief.Retry.MaxAttempts,
		cfg.DailyBrief.Retry.BackoffMinutes,
	)
	processor := dailybriefschedule.NewProcessor(
		runtime.Orchestrator,
		subscriptionRepo,
		issueRepo,
		generationRunRepo,
		retryPolicy,
		metrics,
		time.Now,
	)
	runtime.ScheduleJob = dailybriefschedule.NewJob(
		subscriptionRepo,
		lockManager,
		processor,
		metrics,
		cfg.DailyBrief.Schedule.BatchSize,
		scheduleRunTimeout(cfg),
		time.Now,
	)

	runtime.startScheduleLoop(cfg)
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
	if r.ownsDB && r.DB != nil {
		if err := closeGormDB(r.DB); err != nil && firstErr == nil {
			firstErr = err
		}
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
				log.Errorf("daily brief schedule loop panic recovered: %v", recovered)
			}
		}()

		ticker := time.NewTicker(delay)
		defer ticker.Stop()

		run := func() {
			runCtx, runCancel := context.WithTimeout(ctx, scheduleRunTimeout(cfg))
			defer runCancel()
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Errorf("daily brief schedule loop tick panic recovered: %v", recovered)
				}
			}()
			if err := r.ScheduleJob.Scan(runCtx); err != nil {
				log.Warnf("daily brief schedule scan failed: %v", err)
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

func scheduleScanInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.DailyBrief.Schedule.ScanDelayMs <= 0 {
		return 10 * time.Second
	}
	return time.Duration(cfg.DailyBrief.Schedule.ScanDelayMs) * time.Millisecond
}

func scheduleRunTimeout(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.DailyBrief.Schedule.RunTimeoutMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(cfg.DailyBrief.Schedule.RunTimeoutMs) * time.Millisecond
}
