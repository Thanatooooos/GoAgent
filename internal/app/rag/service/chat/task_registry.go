package chat

import (
	"context"
	"strings"
	"sync"

	aichat "local/rag-project/internal/infra-ai/chat"
)

type ragChatTask struct {
	ctx       context.Context
	cancelFn  context.CancelFunc
	mu        sync.Mutex
	handle    aichat.StreamCancellationHandle
	cancelled bool

	cancelOnce sync.Once
	cancelCh   chan struct{}
	doneCh     chan ragChatTaskResult
}

type ragChatTaskResult struct {
	cancelled   bool
	content     string
	thinking    string
	err         error
	tokenUsage  aichat.TokenUsage
	usageSource string
}

type TaskRegistry struct {
	mu    sync.Mutex
	tasks map[string]*ragChatTask
}

func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{
		tasks: map[string]*ragChatTask{},
	}
}

func (r *TaskRegistry) New(parent context.Context) *ragChatTask {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &ragChatTask{
		ctx:      ctx,
		cancelFn: cancel,
		cancelCh: make(chan struct{}),
		doneCh:   make(chan ragChatTaskResult, 1),
	}
}

func (r *TaskRegistry) Set(taskID string, task *ragChatTask, handle aichat.StreamCancellationHandle) {
	var cancelHandle aichat.StreamCancellationHandle
	r.mu.Lock()
	if task != nil {
		task.mu.Lock()
		task.handle = handle
		if task.cancelled {
			cancelHandle = handle
		}
		task.mu.Unlock()
	}
	r.tasks[taskID] = task
	r.mu.Unlock()
	if cancelHandle != nil {
		cancelHandle.Cancel()
	}
}

func (r *TaskRegistry) Delete(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, taskID)
}

func (r *TaskRegistry) Cancel(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	r.mu.Lock()
	task, ok := r.tasks[taskID]
	r.mu.Unlock()
	if !ok || task == nil {
		return false
	}
	task.cancelOnce.Do(func() {
		task.mu.Lock()
		task.cancelled = true
		handle := task.handle
		task.mu.Unlock()
		close(task.cancelCh)
		if task.cancelFn != nil {
			task.cancelFn()
		}
		if handle != nil {
			handle.Cancel()
		}
	})
	return true
}

func (t *ragChatTask) Context() context.Context {
	if t == nil || t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}
