package schedule_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/schedule"
)

func TestRetryPolicyShouldRetryRespectsBackoffAndMaxAttempts(t *testing.T) {
	t.Parallel()

	policy := schedule.NewRetryPolicy(2, 30)
	finishedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)

	if policy.ShouldRetry(0, finishedAt, finishedAt.Add(5*time.Minute)) {
		t.Fatal("expected first retry to wait at least 10 minutes")
	}
	if !policy.ShouldRetry(0, finishedAt, finishedAt.Add(11*time.Minute)) {
		t.Fatal("expected first retry to be eligible after backoff")
	}
	if policy.ShouldRetry(1, finishedAt, finishedAt.Add(29*time.Minute)) {
		t.Fatal("expected second retry to wait for configured fallback backoff")
	}
	if !policy.ShouldRetry(1, finishedAt, finishedAt.Add(31*time.Minute)) {
		t.Fatal("expected second retry to be eligible after fallback backoff")
	}
	if policy.ShouldRetry(2, finishedAt, finishedAt.Add(24*time.Hour)) {
		t.Fatal("expected max attempts to stop retries")
	}
}

func TestRetryPolicyAllowsConfiguredNumberOfRetryAttempts(t *testing.T) {
	t.Parallel()

	policy := schedule.NewRetryPolicy(2, 30)
	finishedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)

	if !policy.ShouldRetry(0, finishedAt, finishedAt.Add(11*time.Minute)) {
		t.Fatal("expected first retry attempt to be allowed when no retry runs exist yet")
	}
	if !policy.ShouldRetry(1, finishedAt, finishedAt.Add(31*time.Minute)) {
		t.Fatal("expected second retry attempt to be allowed when one retry run already exists")
	}
	if policy.ShouldRetry(2, finishedAt, finishedAt.Add(3*time.Hour)) {
		t.Fatal("expected retries to stop after configured retry attempts are exhausted")
	}
}
