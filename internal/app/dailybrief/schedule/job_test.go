package schedule_test

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	"local/rag-project/internal/app/dailybrief/schedule"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
)

func TestJobScanRecordsSubscribedUsersScanned(t *testing.T) {
	t.Parallel()

	metrics := dailybriefservice.NewMetricsService()
	subscriptionRepo := &jobSubscriptionRepo{
		subscriptions: []domain.Subscription{
			domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"}),
			domain.NewSubscription("user-2", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"}),
		},
	}
	lockManager := schedule.NewLockManager(subscriptionRepo, 900, time.Now)
	processor := schedule.NewProcessor(
		&noopRunner{},
		subscriptionRepo,
		&jobIssueRepo{},
		&jobGenerationRunRepo{},
		schedule.NewRetryPolicy(2, 30),
		metrics,
		time.Now,
	)
	job := schedule.NewJob(subscriptionRepo, lockManager, processor, metrics, 10, time.Second, time.Now)

	if err := job.Scan(context.Background()); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	job.Close()

	snapshot := metrics.Snapshot()
	if snapshot.SubscribedUsersScanned != 2 {
		t.Fatalf("expected 2 scanned users, got %d", snapshot.SubscribedUsersScanned)
	}
}

type noopRunner struct{}

func (n *noopRunner) Run(ctx context.Context, request dailybriefservice.GenerationRequest) (dailybriefservice.GenerationOutcome, error) {
	return dailybriefservice.GenerationOutcome{}, nil
}

type jobSubscriptionRepo struct {
	subscriptions []domain.Subscription
}

func (s *jobSubscriptionRepo) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	return subscription, nil
}

func (s *jobSubscriptionRepo) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	return domain.Subscription{}, nil
}

func (s *jobSubscriptionRepo) List(ctx context.Context, filter port.SubscriptionListFilter) ([]domain.Subscription, error) {
	return append([]domain.Subscription(nil), s.subscriptions...), nil
}

func (s *jobSubscriptionRepo) TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return true, nil
}

func (s *jobSubscriptionRepo) RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error) {
	return true, nil
}

func (s *jobSubscriptionRepo) ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error) {
	return true, nil
}

type jobIssueRepo struct{}

func (s *jobIssueRepo) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *jobIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *jobIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	return domain.Issue{}, nil
}

func (s *jobIssueRepo) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	return domain.Issue{}, nil
}

func (s *jobIssueRepo) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	return nil, nil
}

type jobGenerationRunRepo struct{}

func (s *jobGenerationRunRepo) Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	return run, nil
}

func (s *jobGenerationRunRepo) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	return run, nil
}

func (s *jobGenerationRunRepo) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	return domain.GenerationRun{}, nil
}

func (s *jobGenerationRunRepo) GetLatestFailedByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.GenerationRun, error) {
	return domain.GenerationRun{}, nil
}

func (s *jobGenerationRunRepo) ListRetryEligible(ctx context.Context, filter port.GenerationRunRetryEligibleFilter) ([]domain.GenerationRun, error) {
	return nil, nil
}

func (s *jobGenerationRunRepo) CountRetryRunsByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (int, error) {
	return 0, nil
}
