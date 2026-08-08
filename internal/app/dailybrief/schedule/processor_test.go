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

type stubRunner struct {
	calls int
}

func (s *stubRunner) Run(ctx context.Context, request dailybriefservice.GenerationRequest) (dailybriefservice.GenerationOutcome, error) {
	s.calls++
	return dailybriefservice.GenerationOutcome{}, nil
}

type stubSubscriptionRepo struct {
	subscription domain.Subscription
}

func (s *stubSubscriptionRepo) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	return subscription, nil
}

func (s *stubSubscriptionRepo) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	return s.subscription, nil
}

func (s *stubSubscriptionRepo) List(ctx context.Context, filter port.SubscriptionListFilter) ([]domain.Subscription, error) {
	return nil, nil
}

func (s *stubSubscriptionRepo) TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return true, nil
}

func (s *stubSubscriptionRepo) RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error) {
	return true, nil
}

func (s *stubSubscriptionRepo) ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error) {
	return true, nil
}

type stubIssueRepo struct {
	issue domain.Issue
}

func (s *stubIssueRepo) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *stubIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *stubIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	return s.issue, nil
}

func (s *stubIssueRepo) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	if s.issue.UserID == userID && s.issue.BriefDate == briefDate {
		return s.issue, nil
	}
	return domain.Issue{}, nil
}

func (s *stubIssueRepo) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	return nil, nil
}

type stubGenerationRunRepo struct {
	retryCount      int
	latestFailedRun domain.GenerationRun
}

func (s *stubGenerationRunRepo) Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	return run, nil
}

func (s *stubGenerationRunRepo) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	return run, nil
}

func (s *stubGenerationRunRepo) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	return domain.GenerationRun{}, nil
}

func (s *stubGenerationRunRepo) GetLatestFailedByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.GenerationRun, error) {
	if s.latestFailedRun.UserID == userID && s.latestFailedRun.BriefDate == briefDate {
		return s.latestFailedRun, nil
	}
	return domain.GenerationRun{}, nil
}

func (s *stubGenerationRunRepo) ListRetryEligible(ctx context.Context, filter port.GenerationRunRetryEligibleFilter) ([]domain.GenerationRun, error) {
	return nil, nil
}

func (s *stubGenerationRunRepo) CountRetryRunsByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (int, error) {
	if s.retryCount > 0 {
		return s.retryCount, nil
	}
	return 0, nil
}

func TestProcessorSkipsReadyIssueForScheduledTrigger(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	processor := schedule.NewProcessor(
		runner,
		&stubSubscriptionRepo{subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})},
		&stubIssueRepo{issue: domain.Issue{ID: "issue-1", UserID: "user-1", BriefDate: "2026-06-29", Status: domain.IssueStatusReady}},
		&stubGenerationRunRepo{},
		schedule.NewRetryPolicy(2, 30),
		nil,
		func() time.Time { return now },
	)

	err := processor.ProcessSubscription(context.Background(), domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"}), domain.GenerationRunTriggerTypeScheduled)
	if err != nil {
		t.Fatalf("ProcessSubscription returned error: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", runner.calls)
	}
}

func TestProcessorSkipsFailedIssueForScheduledTrigger(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})
	processor := schedule.NewProcessor(
		runner,
		&stubSubscriptionRepo{subscription: subscription},
		&stubIssueRepo{issue: domain.Issue{ID: "issue-1", UserID: "user-1", BriefDate: "2026-06-29", Status: domain.IssueStatusFailed}},
		&stubGenerationRunRepo{},
		schedule.NewRetryPolicy(2, 30),
		nil,
		func() time.Time { return now },
	)

	err := processor.ProcessSubscription(context.Background(), subscription, domain.GenerationRunTriggerTypeScheduled)
	if err != nil {
		t.Fatalf("ProcessSubscription returned error: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected scheduled trigger to skip failed issue and wait for retry, got %d calls", runner.calls)
	}
}

func TestProcessorRunsWhenDeliveryWindowOpenAndNoIssue(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})
	processor := schedule.NewProcessor(
		runner,
		&stubSubscriptionRepo{subscription: subscription},
		&stubIssueRepo{},
		&stubGenerationRunRepo{},
		schedule.NewRetryPolicy(2, 30),
		nil,
		func() time.Time { return now },
	)

	err := processor.ProcessSubscription(context.Background(), subscription, domain.GenerationRunTriggerTypeScheduled)
	if err != nil {
		t.Fatalf("ProcessSubscription returned error: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
}

func TestProcessorRecordsRetryAttempt(t *testing.T) {
	t.Parallel()

	metrics := dailybriefservice.NewMetricsService()
	finishedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	now := finishedAt.Add(11 * time.Minute)
	run := domain.GenerationRun{
		ID:          "run-1",
		UserID:      "user-1",
		BriefDate:   "2026-06-29",
		Status:      domain.GenerationRunStatusFailed,
		TriggerType: domain.GenerationRunTriggerTypeScheduled,
		FinishedAt:  &finishedAt,
	}
	runRepo := &stubGenerationRunRepo{retryCount: 0}
	processor := schedule.NewProcessor(
		&stubRunner{},
		&stubSubscriptionRepo{subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})},
		&stubIssueRepo{},
		runRepo,
		schedule.NewRetryPolicy(2, 30),
		metrics,
		func() time.Time { return now },
	)

	if err := processor.ProcessRetry(context.Background(), run); err != nil {
		t.Fatalf("ProcessRetry returned error: %v", err)
	}
	if metrics.Snapshot().RetryAttempts != 1 {
		t.Fatalf("expected one retry attempt metric, got %+v", metrics.Snapshot())
	}
}

func TestProcessorSkipsRetryWhenLocalBriefDateWindowHasPassed(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	finishedAt := time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)
	run := domain.GenerationRun{
		ID:          "run-1",
		UserID:      "user-1",
		BriefDate:   "2026-06-29",
		Status:      domain.GenerationRunStatusFailed,
		TriggerType: domain.GenerationRunTriggerTypeScheduled,
		FinishedAt:  &finishedAt,
	}
	processor := schedule.NewProcessor(
		runner,
		&stubSubscriptionRepo{subscription: domain.NewSubscription("user-1", "Asia/Tokyo", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})},
		&stubIssueRepo{},
		&stubGenerationRunRepo{retryCount: 0},
		schedule.NewRetryPolicy(2, 30),
		dailybriefservice.NewMetricsService(),
		func() time.Time { return now },
	)

	if err := processor.ProcessRetry(context.Background(), run); err != nil {
		t.Fatalf("ProcessRetry returned error: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected retry to stop after local brief_date window closed, got %d calls", runner.calls)
	}
}

func TestProcessorAnchorsRetryBackoffToLatestFailedAttempt(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	olderFinishedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	latestFinishedAt := time.Date(2026, 6, 29, 8, 8, 0, 0, time.UTC)
	now := time.Date(2026, 6, 29, 8, 12, 0, 0, time.UTC)
	run := domain.GenerationRun{
		ID:          "scheduled-run",
		UserID:      "user-1",
		BriefDate:   "2026-06-29",
		Status:      domain.GenerationRunStatusFailed,
		TriggerType: domain.GenerationRunTriggerTypeScheduled,
		FinishedAt:  &olderFinishedAt,
	}
	runRepo := &stubGenerationRunRepo{
		retryCount: 1,
		latestFailedRun: domain.GenerationRun{
			ID:          "retry-run-1",
			UserID:      "user-1",
			BriefDate:   "2026-06-29",
			Status:      domain.GenerationRunStatusFailed,
			TriggerType: domain.GenerationRunTriggerTypeRetry,
			FinishedAt:  &latestFinishedAt,
		},
	}
	processor := schedule.NewProcessor(
		runner,
		&stubSubscriptionRepo{subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})},
		&stubIssueRepo{},
		runRepo,
		schedule.NewRetryPolicy(2, 30),
		dailybriefservice.NewMetricsService(),
		func() time.Time { return now },
	)

	if err := processor.ProcessRetry(context.Background(), run); err != nil {
		t.Fatalf("ProcessRetry returned error: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected retry to wait for latest failed attempt backoff, got %d calls", runner.calls)
	}
}

func TestProcessorRetryReturnsErrorWhenDependenciesMissing(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	run := domain.GenerationRun{
		ID:          "run-1",
		UserID:      "user-1",
		BriefDate:   "2026-06-29",
		Status:      domain.GenerationRunStatusFailed,
		TriggerType: domain.GenerationRunTriggerTypeScheduled,
		FinishedAt:  &finishedAt,
	}
	processor := schedule.NewProcessor(
		&stubRunner{},
		nil,
		&stubIssueRepo{},
		nil,
		nil,
		nil,
		func() time.Time { return finishedAt.Add(11 * time.Minute) },
	)

	err := processor.ProcessRetry(context.Background(), run)
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
}
