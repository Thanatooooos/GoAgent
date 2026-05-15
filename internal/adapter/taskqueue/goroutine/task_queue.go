package goroutine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/log"
)

// chunkDocumentProcessor is the interface the task queue needs from the chunk
// processing service. It was previously defined in the deleted rocketmq package.
type chunkDocumentProcessor interface {
	ExecuteChunk(ctx context.Context, input service.ExecuteChunkInput) error
}

// TaskQueue is an in-process task queue backed by goroutines with a semaphore
// for concurrency control. It replaces the RocketMQ-based adapter for single-
// process deployments where the producer and consumer run in the same binary.
type TaskQueue struct {
	processor      chunkDocumentProcessor
	maxConcurrency int
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	sem            chan struct{}
}

var _ port.TaskQueue = (*TaskQueue)(nil)

func NewTaskQueue(processor chunkDocumentProcessor, maxConcurrency int) *TaskQueue {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &TaskQueue{
		processor:      processor,
		maxConcurrency: maxConcurrency,
		ctx:            ctx,
		cancel:         cancel,
		sem:            make(chan struct{}, maxConcurrency),
	}
}

func (q *TaskQueue) SubmitChunkDocument(ctx context.Context, task port.ChunkDocumentTask) error {
	if q == nil || q.processor == nil {
		return fmt.Errorf("goroutine task queue: chunk document processor is required")
	}
	taskID := strings.TrimSpace(task.TaskID)
	documentID := strings.TrimSpace(task.DocumentID)
	if taskID == "" {
		return fmt.Errorf("goroutine task queue: chunk document task id is required")
	}
	if documentID == "" {
		return fmt.Errorf("goroutine task queue: chunk document id is required")
	}

	select {
	case q.sem <- struct{}{}:
	case <-q.ctx.Done():
		return q.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}

	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		defer func() { <-q.sem }()
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("goroutine task queue: chunk task panic recovered: taskId=%s documentId=%s panic=%v", taskID, documentID, r)
			}
		}()

		if err := q.processor.ExecuteChunk(ctx, service.ExecuteChunkInput{
			DocumentID:  documentID,
			TriggeredBy: strings.TrimSpace(task.TriggeredBy),
		}); err != nil {
			log.Errorf("goroutine task queue: chunk task failed: taskId=%s documentId=%s err=%v", taskID, documentID, err)
		}
	}()
	return nil
}

func (q *TaskQueue) Shutdown() {
	if q == nil {
		return
	}
	q.cancel()
	q.wg.Wait()
}
