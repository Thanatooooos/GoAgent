package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"

	"local/rag-project/internal/framework/log"
)

// AsynqRuntimeOptions configures the Redis-backed ingestion worker.
type AsynqRuntimeOptions struct {
	Redis        asynq.RedisClientOpt
	QueueName    string
	Concurrency  int
	MaxRetries   int
	RetryBackoff time.Duration
}

// AsynqRuntime owns the persistent queue client and its local worker server.
type AsynqRuntime struct {
	Queue     *AsynqTaskQueue
	server    *asynq.Server
	client    *asynq.Client
	processor *AsynqProcessor
	once      sync.Once
}

func NewAsynqRuntime(options AsynqRuntimeOptions, processor *AsynqProcessor) (*AsynqRuntime, error) {
	if processor == nil {
		return nil, fmt.Errorf("asynq ingestion processor is required")
	}
	queueName := strings.TrimSpace(options.QueueName)
	if queueName == "" {
		queueName = "ingestion"
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 1
	}
	if options.MaxRetries < 0 {
		options.MaxRetries = 0
	}
	client := asynq.NewClient(options.Redis)
	serverConfig := asynq.Config{
		Concurrency: options.Concurrency,
		Queues:      map[string]int{queueName: 1},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			retried, _ := asynq.GetRetryCount(ctx)
			maxRetries, _ := asynq.GetMaxRetry(ctx)
			log.Errorw("ingestion queue task failed",
				"taskType", task.Type(),
				"retryCount", retried,
				"maxRetries", maxRetries,
				"retryExhausted", retried >= maxRetries,
				"error", err.Error(),
			)
		}),
	}
	if options.RetryBackoff > 0 {
		serverConfig.RetryDelayFunc = func(retryCount int, _ error, _ *asynq.Task) time.Duration {
			return options.RetryBackoff * time.Duration(retryCount+1)
		}
	}
	server := asynq.NewServer(options.Redis, serverConfig)
	return &AsynqRuntime{
		Queue:     newAsynqTaskQueue(client, queueName, options.MaxRetries),
		server:    server,
		client:    client,
		processor: processor,
	}, nil
}

// Start checks Redis connectivity before accepting work, then starts the worker.
func (r *AsynqRuntime) Start() error {
	if r == nil || r.server == nil || r.client == nil || r.processor == nil {
		return fmt.Errorf("asynq ingestion runtime dependencies are required")
	}
	if err := r.client.Ping(); err != nil {
		return fmt.Errorf("ping ingestion queue redis: %w", err)
	}
	mux := asynq.NewServeMux()
	if err := r.processor.Register(mux); err != nil {
		return err
	}
	return r.server.Start(mux)
}

func (r *AsynqRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		if r.server != nil {
			r.server.Shutdown()
		}
	})
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
