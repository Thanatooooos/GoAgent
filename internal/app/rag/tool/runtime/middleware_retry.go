package runtime

import (
	"context"
	"math"
	"strings"
	"time"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

// RetryMiddleware retries failed tool calls with exponential backoff.
// Only retries on transient errors (context deadline exceeded, connection refused, temporary network failures).
// Non-transient errors (validation, not found) are returned immediately.
type RetryMiddleware struct {
	MaxRetries int
	BaseDelay  time.Duration
}

func (m *RetryMiddleware) Wrap(next ToolHandler) ToolHandler {
	maxRetries := m.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	baseDelay := m.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}

	return func(ctx context.Context, call Call) (Result, error) {
		var lastResult Result
		var lastErr error

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
				log.Infof("[tool] %s retry %d/%d after %v", call.Name, attempt, maxRetries, delay)

				select {
				case <-ctx.Done():
					return lastResult, ctx.Err()
				case <-time.After(delay):
				}
			}

			result, err := next(ctx, call)

			if err == nil && strings.TrimSpace(result.ErrorMessage) == "" {
				return result, nil
			}

			lastResult = result
			lastErr = err

			if !isTransient(err, result.ErrorMessage) {
				return result, err
			}
		}

		return lastResult, lastErr
	}
}

func isTransient(err error, errMsg string) bool {
	if err == nil {
		return strings.TrimSpace(errMsg) != ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "temporary failure"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "eof"),
		strings.Contains(msg, "broken pipe"):
		return true
	}
	if strings.TrimSpace(errMsg) != "" {
		lower := strings.ToLower(errMsg)
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "temporary") {
			return true
		}
	}
	return false
}
