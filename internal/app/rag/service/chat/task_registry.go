package chat

import (
	"strings"
	"sync"

	aichat "local/rag-project/internal/infra-ai/chat"
)

type ragChatTask struct {
	handle aichat.StreamCancellationHandle

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

func (r *TaskRegistry) New() *ragChatTask {
	return &ragChatTask{
		cancelCh: make(chan struct{}),
		doneCh:   make(chan ragChatTaskResult, 1),
	}
}

func (r *TaskRegistry) Set(taskID string, task *ragChatTask, handle aichat.StreamCancellationHandle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if task != nil {
		task.handle = handle
	}
	r.tasks[taskID] = task
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
		close(task.cancelCh)
		if task.handle != nil {
			task.handle.Cancel()
		}
	})
	return true
}
