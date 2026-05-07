package schedule

import (
	"context"
	"fmt"
	"sync"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/log"
)

const (
	defaultScheduleBatchSize             = 20
	defaultRunningDocumentTimeoutMinutes = int64(30)
)

type ScheduleLeaseProcessor interface {
	Process(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error
}

type ScheduleTaskDispatcher interface {
	Submit(task func()) error
}

type KnowledgeDocumentScheduleJobOptions struct {
	LockManager           *ScheduleLockManager
	Processor             ScheduleLeaseProcessor
	Dispatcher            ScheduleTaskDispatcher
	BatchSize             int
	RunningTimeoutMinutes int64
	Now                   func() time.Time
}

// KnowledgeDocumentScheduleJob polls due schedules and hands execution off to a worker.
type KnowledgeDocumentScheduleJob struct {
	scheduleRepo                 port.KnowledgeDocumentScheduleRepository
	lockManager                  *ScheduleLockManager
	processor                    ScheduleLeaseProcessor
	dispatcher                   ScheduleTaskDispatcher
	documentStatusHelper         DocumentStatusHelper
	batchSize                    int
	runningDocumentTimeoutMinute int64
	now                          func() time.Time
	dispatcherCancel             context.CancelFunc
	dispatcherWG                 *sync.WaitGroup
}

func NewKnowledgeDocumentScheduleJob(scheduleRepo port.KnowledgeDocumentScheduleRepository, documentHelper DocumentStatusHelper) *KnowledgeDocumentScheduleJob {
	return NewKnowledgeDocumentScheduleJobWithOptions(scheduleRepo, documentHelper, KnowledgeDocumentScheduleJobOptions{})
}

func NewKnowledgeDocumentScheduleJobWithOptions(
	scheduleRepo port.KnowledgeDocumentScheduleRepository,
	documentHelper DocumentStatusHelper,
	options KnowledgeDocumentScheduleJobOptions,
) *KnowledgeDocumentScheduleJob {
	lockManager := options.LockManager
	if lockManager == nil && scheduleRepo != nil {
		lockManager = NewScheduleLockManager(scheduleRepo, ScheduleLockOptions{})
	}

	dispatcher, cancel, wg := normalizeScheduleTaskDispatcher(options.Dispatcher)

	return &KnowledgeDocumentScheduleJob{
		scheduleRepo:                 scheduleRepo,
		lockManager:                  lockManager,
		processor:                    options.Processor,
		dispatcher:                   dispatcher,
		documentStatusHelper:         documentHelper,
		batchSize:                    normalizePositiveInt(options.BatchSize, defaultScheduleBatchSize),
		runningDocumentTimeoutMinute: normalizePositiveInt64(options.RunningTimeoutMinutes, defaultRunningDocumentTimeoutMinutes),
		now:                          normalizeClock(options.Now),
		dispatcherCancel:             cancel,
		dispatcherWG:                 wg,
	}
}

func (j *KnowledgeDocumentScheduleJob) ListDue(ctx context.Context, now time.Time, limit int) error {
	if j == nil || j.scheduleRepo == nil {
		return nil
	}
	_, err := j.scheduleRepo.ListDue(ctx, now, limit)
	return err
}

func (j *KnowledgeDocumentScheduleJob) RecoverStuckRunningDocuments(ctx context.Context) error {
	if j == nil {
		return nil
	}
	result, err := j.documentStatusHelper.RecoverStuckRunning(ctx, j.runningDocumentTimeoutMinute)
	if result > 0 {
		log.Warnf("recover %d stuck running knowledge documents to failed", result)
	}
	return err
}

func (j *KnowledgeDocumentScheduleJob) Scan(ctx context.Context) error {
	if j == nil || j.scheduleRepo == nil {
		return nil
	}
	if j.lockManager == nil {
		return fmt.Errorf("knowledge document schedule lock manager is required")
	}

	now := j.now()
	schedules, err := j.scheduleRepo.ListDue(ctx, now, j.batchSize)
	if err != nil {
		return err
	}

	for _, item := range schedules {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.ID == "" {
			continue
		}

		lease, acquired, err := j.lockManager.TryAcquire(ctx, item.ID, now)
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}

		if err := j.dispatchLease(ctx, lease); err != nil {
			if _, releaseErr := j.lockManager.Release(ctx, lease); releaseErr != nil {
				log.Warnf("release schedule lock after dispatch failure failed: scheduleId=%s lockToken=%s err=%v",
					lease.ScheduleID, lease.LockToken, releaseErr)
			}
			return err
		}
	}

	return nil
}

func (j *KnowledgeDocumentScheduleJob) scan(ctx context.Context) error {
	return j.Scan(ctx)
}

// Close 停止默认任务调度器并等待已提交任务退出。
func (j *KnowledgeDocumentScheduleJob) Close() {
	if j == nil {
		return
	}
	if j.dispatcherCancel != nil {
		j.dispatcherCancel()
	}
	if j.dispatcherWG != nil {
		j.dispatcherWG.Wait()
	}
}

func (j *KnowledgeDocumentScheduleJob) dispatchLease(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error {
	if j.processor == nil {
		_, err := j.lockManager.Release(ctx, lease)
		return err
	}

	return j.dispatcher.Submit(func() {
		defer func() {
			releaseCtx, releaseCancel := newBackgroundTaskContext(ctx, 5*time.Second)
			defer releaseCancel()
			if _, err := j.lockManager.Release(releaseCtx, lease); err != nil {
				log.Warnf("release schedule lock after processing failed: scheduleId=%s lockToken=%s err=%v",
					lease.ScheduleID, lease.LockToken, err)
			}
		}()

		processCtx := ctx
		if managed, ok := j.dispatcher.(*managedScheduleTaskDispatcher); ok && managed.ctx != nil {
			processCtx = managed.ctx
		}
		if err := j.processor.Process(processCtx, lease); err != nil {
			log.Errorf("process knowledge document schedule failed: scheduleId=%s lockToken=%s err=%v",
				lease.ScheduleID, lease.LockToken, err)
		}
	})
}

type managedScheduleTaskDispatcher struct {
	ctx context.Context
	wg  *sync.WaitGroup
}

func (d *managedScheduleTaskDispatcher) Submit(task func()) error {
	if d == nil || d.ctx == nil || d.wg == nil {
		return fmt.Errorf("managed schedule task dispatcher is not initialized")
	}
	if d.ctx.Err() != nil {
		return d.ctx.Err()
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("schedule task dispatch panic recovered: %v", recovered)
			}
		}()
		if d.ctx.Err() != nil {
			return
		}
		task()
	}()
	return nil
}

func normalizeScheduleTaskDispatcher(dispatcher ScheduleTaskDispatcher) (ScheduleTaskDispatcher, context.CancelFunc, *sync.WaitGroup) {
	if dispatcher != nil {
		return dispatcher, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	return &managedScheduleTaskDispatcher{
		ctx: ctx,
		wg:  wg,
	}, cancel, wg
}

func normalizePositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func normalizePositiveInt64(value int64, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func newBackgroundTaskContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, timeout)
}
