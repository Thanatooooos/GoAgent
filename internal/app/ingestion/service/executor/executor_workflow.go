package executor

import (
	ingestionobserver "local/rag-project/internal/app/ingestion/service/observer"
	ingestionrunner "local/rag-project/internal/app/ingestion/service/runner"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
)

const defaultExecutorMaxConcurrent = 8

// ExecutorServiceOptions 描述执行编排服务所需依赖。
type ExecutorServiceOptions struct {
	TaskRepo        port.TaskRepository
	TaskNodeRepo    port.TaskNodeRepository
	WorkflowBuilder ingestionworkflow.WorkflowBuilder
	NodeRunners     *ingestionrunner.NodeRunnerRegistry
	TaskObserver    ingestionobserver.TaskObserver
	Metrics         *ingestionobserver.MetricsService
	MaxConcurrent   int
	MaxRetries      int
	RetryBackoffMs  int
}

// ExecutorService 负责 ingestion task 的执行编排入口。
type ExecutorService struct {
	taskRepo        port.TaskRepository
	taskNodeRepo    port.TaskNodeRepository
	workflowBuilder ingestionworkflow.WorkflowBuilder
	graphExecutor   *EinoGraphExecutor
	nodeRunners     *ingestionrunner.NodeRunnerRegistry
	taskObserver    ingestionobserver.TaskObserver
	metrics         *ingestionobserver.MetricsService
	now             func() time.Time
	maxRetries      int
	retryBackoffMs  int

	asyncCtx    context.Context
	asyncCancel context.CancelFunc
	slots       chan struct{}
	wg          sync.WaitGroup

	mu     sync.RWMutex
	closed bool
}

// NewExecutorService 创建执行编排服务。
func NewExecutorService(options ExecutorServiceOptions) *ExecutorService {
	builder := options.WorkflowBuilder
	if builder == nil {
		builder = ingestionworkflow.NewEinoGraphWorkflowBuilder()
	}

	maxConcurrent := options.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultExecutorMaxConcurrent
	}
	maxRetries := options.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 5 {
		maxRetries = 5 // 重试上限保护
	}
	retryBackoffMs := options.RetryBackoffMs
	if retryBackoffMs <= 0 {
		retryBackoffMs = 1000
	}
	asyncCtx, asyncCancel := context.WithCancel(context.Background())

	service := &ExecutorService{
		taskRepo:        options.TaskRepo,
		taskNodeRepo:    options.TaskNodeRepo,
		workflowBuilder: builder,
		nodeRunners:     options.NodeRunners,
		taskObserver:    options.TaskObserver,
		metrics:         options.Metrics,
		now:             time.Now,
		maxRetries:      maxRetries,
		retryBackoffMs:  retryBackoffMs,
		asyncCtx:        asyncCtx,
		asyncCancel:     asyncCancel,
		slots:           make(chan struct{}, maxConcurrent),
	}
	service.graphExecutor = NewEinoGraphExecutor(service)
	return service
}

// Submit 提供 task 提交边界，并完成最小编排准备。
func (s *ExecutorService) Submit(ctx context.Context, pipeline domain.Pipeline, task domain.Task) error {
	if s == nil {
		return exception.NewServiceException("ingestion executor service is required", nil)
	}
	if s.workflowBuilder == nil {
		return exception.NewServiceException("ingestion workflow builder is required", nil)
	}
	if strings.TrimSpace(task.ID) == "" {
		return exception.NewClientException("task id is required", nil)
	}
	if strings.TrimSpace(pipeline.ID) == "" {
		return exception.NewClientException("pipeline id is required", nil)
	}

	workflow, err := s.workflowBuilder.Build(ctx, pipeline, task)
	if err != nil {
		return err
	}
	if len(workflow.NodeOrder) == 0 {
		return exception.NewClientException("ingestion workflow nodes are required", nil)
	}

	state := s.newExecutionState(task, pipeline)
	return s.startWorkflow(workflow, state)
}

// BuildWorkflow 暴露给后续 EINO 适配层使用的工作流构建入口。
func (s *ExecutorService) BuildWorkflow(ctx context.Context, pipeline domain.Pipeline, task domain.Task) (ingestionworkflow.WorkflowSpec, error) {
	if s == nil || s.workflowBuilder == nil {
		return ingestionworkflow.WorkflowSpec{}, exception.NewServiceException("ingestion workflow builder is required", nil)
	}
	return s.workflowBuilder.Build(ctx, pipeline, task)
}

// RunnerForNode 返回某个节点类型对应的运行器。
func (s *ExecutorService) RunnerForNode(nodeType string) (ingestionrunner.NodeRunner, bool) {
	if s == nil || s.nodeRunners == nil {
		return nil, false
	}
	return s.nodeRunners.Get(strings.TrimSpace(nodeType))
}

// Observer 返回当前装配的 task observer。
func (s *ExecutorService) Observer() ingestionobserver.TaskObserver {
	if s == nil {
		return nil
	}
	return s.taskObserver
}

// SetTaskObserver 更新当前执行器使用的 task observer。
func (s *ExecutorService) SetTaskObserver(observer ingestionobserver.TaskObserver) {
	if s == nil {
		return
	}
	s.taskObserver = observer
}

// Close 停止接收新任务并等待已启动 workflow 结束。
func (s *ExecutorService) Close() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancel := s.asyncCancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

// newExecutionState 为后续 EINO workflow 执行准备共享上下文。
func (s *ExecutorService) newExecutionState(task domain.Task, pipeline domain.Pipeline) ingestionworkflow.ExecutionState {
	return ingestionworkflow.ExecutionState{
		Task:        task,
		Pipeline:    pipeline,
		Artifacts:   map[string]any{},
		NodeOutputs: map[string]map[string]any{},
		StartedAt:   s.now(),
	}
}

func (s *ExecutorService) startWorkflow(workflow ingestionworkflow.WorkflowSpec, state ingestionworkflow.ExecutionState) error {
	if s == nil {
		return exception.NewServiceException("ingestion executor service is required", nil)
	}

	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return exception.NewServiceException("ingestion executor service is closed", nil)
	}

	select {
	case <-s.asyncCtx.Done():
		return exception.NewServiceException("ingestion executor service is shutting down", nil)
	case s.slots <- struct{}{}:
	}
	if s.metrics != nil {
		s.metrics.RecordTaskSubmitted(state.Task)
	}

	log.Infow("ingestion executor acquired slot",
		"taskId", state.Task.ID,
		"runningConcurrency", cap(s.slots)-len(s.slots)+1,
		"maxConcurrent", cap(s.slots),
	)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			<-s.slots
			log.Infow("ingestion executor released slot",
				"taskId", state.Task.ID,
				"runningConcurrency", cap(s.slots)-len(s.slots),
				"maxConcurrent", cap(s.slots),
			)
		}()

		runCtx := s.asyncCtx
		defer func() {
			if recovered := recover(); recovered != nil {
				s.handleWorkflowPanic(runCtx, state, recovered)
			}
		}()

		s.runWorkflow(runCtx, workflow, state)
	}()
	return nil
}

func (s *ExecutorService) handleWorkflowPanic(ctx context.Context, state ingestionworkflow.ExecutionState, recovered any) {
	if s == nil {
		return
	}

	panicErr := exception.NewServiceException(fmt.Sprintf("workflow panicked: %v", recovered), nil)
	log.Errorw("ingestion workflow panicked",
		"taskId", state.Task.ID,
		"pipelineId", state.Task.PipelineID,
		"panic", recovered,
	)
	failedState := state.Clone()
	completedAt := s.now()
	failedTask := failedState.Task
	failedTask.CompletedAt = &completedAt
	failedTask.UpdatedAt = completedAt
	failedState.CompletedAt = &completedAt
	failedState.Error = panicErr
	failedState.Task = failedTask

	if s.taskObserver != nil {
		_ = s.taskObserver.OnTaskCompleted(ctx, failedTask, failedState, panicErr)
	}
}

// runWorkflow 顺序执行最小 workflow 节点链路。
func (s *ExecutorService) runWorkflow(ctx context.Context, workflow ingestionworkflow.WorkflowSpec, state ingestionworkflow.ExecutionState) {
	if s == nil {
		return
	}
	task := state.Task
	startedAt := s.now()
	task.Status = domain.TaskStatusRunning
	task.StartedAt = &startedAt
	task.UpdatedAt = startedAt
	state.Task = task
	log.Infow("ingestion task started",
		"taskId", task.ID,
		"pipelineId", task.PipelineID,
		"sourceType", task.SourceType,
	)
	if s.taskObserver != nil {
		if err := s.taskObserver.OnTaskStarted(ctx, task); err != nil {
			return
		}
	}

	current, execErr := s.graphExecutor.Execute(ctx, workflow, state)

	completedAt := s.now()
	task.CompletedAt = &completedAt
	task.UpdatedAt = completedAt
	current.CompletedAt = &completedAt
	current.Error = execErr
	current.Task = task

	totalDuration := completedAt.Sub(startedAt)
	if execErr != nil {
		log.Errorw("ingestion task failed",
			"taskId", task.ID,
			"pipelineId", task.PipelineID,
			"sourceType", task.SourceType,
			"totalDurationMs", totalDuration.Milliseconds(),
			"error", execErr.Error(),
		)
	} else {
		log.Infow("ingestion task completed",
			"taskId", task.ID,
			"pipelineId", task.PipelineID,
			"sourceType", task.SourceType,
			"totalDurationMs", totalDuration.Milliseconds(),
			"chunkCount", len(current.Chunks),
		)
	}
	if s.taskObserver != nil {
		if err := s.taskObserver.OnTaskCompleted(ctx, task, current, execErr); err != nil {
			log.Errorw("ingestion task completion observer failed",
				"taskId", task.ID,
				"pipelineId", task.PipelineID,
				"error", err.Error(),
			)
		}
	}
}

func (s *ExecutorService) executeWorkflowNode(ctx context.Context, runtime *einoTaskRuntime, item ingestionworkflow.WorkflowNodeSpec) error {
	if s == nil {
		return exception.NewServiceException("ingestion executor service is required", nil)
	}
	select {
	case <-ctx.Done():
		return exception.NewServiceException("workflow execution canceled", ctx.Err())
	default:
	}

	current := runtime.Snapshot()
	runner, ok := s.RunnerForNode(item.Node.NodeType)
	if !ok {
		return exception.NewClientException("node runner not found for node type: "+item.Node.NodeType, nil)
	}
	task := current.Task
	log.Infow("ingestion node started",
		"taskId", task.ID,
		"nodeId", item.Node.NodeID,
		"nodeType", item.Node.NodeType,
		"order", item.Order,
	)
	if s.taskObserver != nil {
		if err := s.taskObserver.OnNodeStarted(ctx, task, item); err != nil {
			return exception.NewServiceException("failed to mark ingestion node running", err)
		}
	}

	nodeRetryCount, nodeRetryBackoffMs := s.resolveNodeRetrySettings(item)
	var nextState ingestionworkflow.ExecutionState
	var output map[string]any
	var runErr error
	var totalDuration time.Duration
	for attempt := 0; attempt <= nodeRetryCount; attempt++ {
		if attempt > 0 {
			var cancelErr error
			select {
			case <-ctx.Done():
				cancelErr = exception.NewServiceException("workflow execution canceled during retry", ctx.Err())
			default:
			}
			if cancelErr != nil {
				runErr = cancelErr
				break
			}
			backoff := time.Duration(nodeRetryBackoffMs*(1<<(attempt-1))) * time.Millisecond
			log.Infow("ingestion node retrying",
				"taskId", task.ID,
				"nodeId", item.Node.NodeID,
				"nodeType", item.Node.NodeType,
				"attempt", attempt,
				"maxRetries", nodeRetryCount,
				"backoffMs", backoff.Milliseconds(),
			)
			if s.taskObserver != nil {
				if err := s.taskObserver.OnNodeRetry(ctx, task, item, attempt, backoff, runErr); err != nil {
					runErr = exception.NewServiceException("failed to record ingestion node retry", err)
					break
				}
			}
			select {
			case <-ctx.Done():
				cancelErr = exception.NewServiceException("workflow execution canceled during retry backoff", ctx.Err())
			case <-time.After(backoff):
			}
			if cancelErr != nil {
				runErr = cancelErr
				break
			}
		}

		attemptState := runtime.Snapshot()
		attemptStart := s.now()
		nextState, output, runErr = runner.Run(ctx, attemptState, item.Node)
		attemptDuration := s.now().Sub(attemptStart)
		totalDuration += attemptDuration
		if runErr == nil {
			break
		}
		log.Errorw("ingestion node attempt failed",
			"taskId", task.ID,
			"nodeId", item.Node.NodeID,
			"nodeType", item.Node.NodeType,
			"attempt", attempt,
			"maxRetries", nodeRetryCount,
			"durationMs", attemptDuration.Milliseconds(),
			"error", runErr.Error(),
		)
	}

	if s.taskObserver != nil {
		if observeErr := s.taskObserver.OnNodeCompleted(ctx, task, item, output, totalDuration, runErr); observeErr != nil && runErr == nil {
			runErr = exception.NewServiceException("failed to persist ingestion node result", observeErr)
		}
	}
	if runErr != nil {
		log.Errorw("ingestion node failed",
			"taskId", task.ID,
			"nodeId", item.Node.NodeID,
			"nodeType", item.Node.NodeType,
			"order", item.Order,
			"durationMs", totalDuration.Milliseconds(),
			"error", runErr.Error(),
		)
		return runErr
	}
	log.Infow("ingestion node completed",
		"taskId", task.ID,
		"nodeId", item.Node.NodeID,
		"nodeType", item.Node.NodeType,
		"order", item.Order,
		"durationMs", totalDuration.Milliseconds(),
	)
	if nextState.NodeOutputs == nil {
		nextState.NodeOutputs = map[string]map[string]any{}
	}
	if output != nil {
		nextState.NodeOutputs[item.Node.NodeID] = output
	}
	syncBuiltInArtifacts(&nextState, item)
	runtime.Commit(item, nextState, output)
	return nil
}

func (s *ExecutorService) resolveNodeRetrySettings(item ingestionworkflow.WorkflowNodeSpec) (int, int) {
	nodeRetryCount := s.maxRetries
	if val, ok := item.Node.Settings["retryCount"].(float64); ok {
		nodeRetryCount = int(val)
	}
	if nodeRetryCount < 0 {
		nodeRetryCount = 0
	}
	if nodeRetryCount > 5 {
		nodeRetryCount = 5
	}
	nodeRetryBackoffMs := s.retryBackoffMs
	if val, ok := item.Node.Settings["retryBackoffMs"].(float64); ok {
		nodeRetryBackoffMs = int(val)
	}
	if nodeRetryBackoffMs <= 0 {
		nodeRetryBackoffMs = 1000
	}
	return nodeRetryCount, nodeRetryBackoffMs
}

// MaxConcurrent 返回执行器最大并发数。
func (s *ExecutorService) MaxConcurrent() int {
	if s == nil {
		return 0
	}
	return cap(s.slots)
}
