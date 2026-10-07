package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// TaskRegistry is the explicit-stop boundary. Transport disconnects never
// touch it; only a Stop request calls Cancel.
type TaskRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewTaskRegistry() *TaskRegistry { return &TaskRegistry{cancels: map[string]context.CancelFunc{}} }

func (r *TaskRegistry) Start(taskID string, parent context.Context) (context.Context, func(), error) {
	if r == nil {
		return nil, nil, fmt.Errorf("runtime task registry is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil, fmt.Errorf("runtime task id is required")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	if _, exists := r.cancels[taskID]; exists {
		r.mu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("runtime task %q is already running", taskID)
	}
	r.cancels[taskID] = cancel
	r.mu.Unlock()
	return ctx, func() {
		cancel()
		r.mu.Lock()
		delete(r.cancels, taskID)
		r.mu.Unlock()
	}, nil
}

func (r *TaskRegistry) Cancel(taskID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	cancel, ok := r.cancels[strings.TrimSpace(taskID)]
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
