package schedule

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/framework/log"
)

type Job struct {
	subscriptionRepo port.SubscriptionRepository
	lockManager      *LockManager
	processor        *Processor
	metrics          *dailybriefservice.MetricsService
	batchSize        int
	runTimeout       time.Duration
	now              func() time.Time

	wg sync.WaitGroup
}

func NewJob(
	subscriptionRepo port.SubscriptionRepository,
	lockManager *LockManager,
	processor *Processor,
	metrics *dailybriefservice.MetricsService,
	batchSize int,
	runTimeout time.Duration,
	now func() time.Time,
) *Job {
	if now == nil {
		now = time.Now
	}
	if batchSize <= 0 {
		batchSize = 20
	}
	if runTimeout <= 0 {
		runTimeout = 30 * time.Second
	}
	return &Job{
		subscriptionRepo: subscriptionRepo,
		lockManager:      lockManager,
		processor:        processor,
		metrics:          metrics,
		batchSize:        batchSize,
		runTimeout:       runTimeout,
		now:              now,
	}
}

func (j *Job) Close() {
	if j == nil {
		return
	}
	j.wg.Wait()
}

func (j *Job) Scan(ctx context.Context) error {
	if j == nil {
		return nil
	}
	now := j.now()
	enabled := true
	subscriptions, err := j.subscriptionRepo.List(ctx, port.SubscriptionListFilter{
		Enabled: &enabled,
		ListOptions: port.ListOptions{
			Limit: j.batchSize,
		},
	})
	if err != nil {
		return err
	}
	if j.metrics != nil {
		j.metrics.RecordSubscribedUsersScanned(len(subscriptions))
	}

	var scanErrors []error
	for _, subscription := range subscriptions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := j.dispatchSubscription(ctx, subscription); err != nil {
			scanErrors = append(scanErrors, err)
		}
	}
	if err := j.scanRetries(ctx, now); err != nil {
		scanErrors = append(scanErrors, err)
	}
	return errors.Join(scanErrors...)
}

func (j *Job) dispatchSubscription(ctx context.Context, subscription domain.Subscription) error {
	lease, acquired, err := j.lockManager.TryAcquire(ctx, subscription.UserID)
	if err != nil {
		return fmt.Errorf("acquire lock for user %s: %w", subscription.UserID, err)
	}
	if !acquired {
		return nil
	}

	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := j.lockManager.Release(releaseCtx, lease); err != nil {
				log.Warnf("release daily brief lock failed: userId=%s err=%v", lease.UserID, err)
			}
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("daily brief schedule panic: userId=%s recovered=%v", subscription.UserID, recovered)
			}
		}()

		workCtx, cancel := context.WithTimeout(context.Background(), j.runTimeout)
		defer cancel()
		if err := j.processor.ProcessSubscription(workCtx, subscription, domain.GenerationRunTriggerTypeScheduled); err != nil {
			log.Warnf("process daily brief subscription failed: userId=%s err=%v", subscription.UserID, err)
		}
	}()
	return nil
}

func (j *Job) scanRetries(ctx context.Context, now time.Time) error {
	if j.processor == nil {
		return nil
	}
	return j.processor.ProcessRetryBatch(ctx, now, j.batchSize)
}
