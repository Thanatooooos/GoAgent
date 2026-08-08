package schedule

import (
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

var defaultRetryBackoff = []time.Duration{
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

type RetryPolicy struct {
	maxAttempts int
	backoff     []time.Duration
}

func NewRetryPolicy(maxAttempts int, fallbackMinutes int) *RetryPolicy {
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	backoff := append([]time.Duration(nil), defaultRetryBackoff...)
	if fallbackMinutes > 0 && len(backoff) > 1 {
		backoff[1] = time.Duration(fallbackMinutes) * time.Minute
	}
	if maxAttempts > len(backoff) {
		last := backoff[len(backoff)-1]
		for len(backoff) < maxAttempts {
			backoff = append(backoff, last)
		}
	} else if maxAttempts < len(backoff) {
		backoff = backoff[:maxAttempts]
	}
	return &RetryPolicy{maxAttempts: maxAttempts, backoff: backoff}
}

func (p *RetryPolicy) MaxAttempts() int {
	if p == nil {
		return 2
	}
	return p.maxAttempts
}

func (p *RetryPolicy) ShouldRetry(retryCount int, finishedAt time.Time, now time.Time) bool {
	if p == nil {
		return false
	}
	if retryCount >= p.maxAttempts {
		return false
	}
	if finishedAt.IsZero() {
		return false
	}
	backoffIndex := retryCount
	if backoffIndex >= len(p.backoff) {
		backoffIndex = len(p.backoff) - 1
	}
	return !now.Before(finishedAt.Add(p.backoff[backoffIndex]))
}

func (p *RetryPolicy) IsRetryTrigger(run domain.GenerationRun) bool {
	return run.TriggerType == domain.GenerationRunTriggerTypeRetry
}
