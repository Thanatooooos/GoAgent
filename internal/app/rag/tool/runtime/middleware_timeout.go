package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

// TimeoutMiddleware enforces a per-tool-call deadline.
// If the tool call exceeds the duration, it is cancelled via context and returns a failed Result.
type TimeoutMiddleware struct {
	Timeout time.Duration
}

func (m *TimeoutMiddleware) Wrap(next ToolHandler) ToolHandler {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return func(ctx context.Context, call Call) (Result, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		type outcome struct {
			result Result
			err    error
		}
		done := make(chan outcome, 1)

		var once sync.Once
		go func() {
			r, err := next(ctx, call)
			once.Do(func() { done <- outcome{r, err} })
		}()

		select {
		case o := <-done:
			return o.result, o.err
		case <-ctx.Done():
			once.Do(func() {}) // prevent goroutine leak on channel send
			name := call.Name
			log.Warnf("[tool] %s timed out after %v", name, timeout)
			return Result{
				Name:         name,
				Status:       CallStatusFailed,
				ErrorMessage: fmt.Sprintf("tool call timed out after %v", timeout),
			}, ctx.Err()
		}
	}
}
