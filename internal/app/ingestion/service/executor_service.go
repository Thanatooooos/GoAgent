package service

import (
	"context"
	"strings"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
	"local/rag-project/internal/framework/exception"
)

// ExecutorServiceOptions 描述执行编排服务所需依赖。
type ExecutorServiceOptions struct {
	TaskRepo        port.TaskRepository
	TaskNodeRepo    port.TaskNodeRepository
	WorkflowBuilder WorkflowBuilder
	NodeRunners     *NodeRunnerRegistry
	TaskObserver    TaskObserver
}

// ExecutorService 负责 ingestion task 的执行编排入口。
type ExecutorService struct {
	taskRepo        port.TaskRepository
	taskNodeRepo    port.TaskNodeRepository
	workflowBuilder WorkflowBuilder
	nodeRunners     *NodeRunnerRegistry
	taskObserver    TaskObserver
	now             func() time.Time
}

// NewExecutorService 创建执行编排服务。
func NewExecutorService(options ExecutorServiceOptions) *ExecutorService {
	builder := options.WorkflowBuilder
	if builder == nil {
		builder = NewLinearWorkflowBuilder()
	}
	return &ExecutorService{
		taskRepo:        options.TaskRepo,
		taskNodeRepo:    options.TaskNodeRepo,
		workflowBuilder: builder,
		nodeRunners:     options.NodeRunners,
		taskObserver:    options.TaskObserver,
		now:             time.Now,
	}
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
	go s.runWorkflow(context.Background(), workflow, state)
	return nil
}

// BuildWorkflow 暴露给后续 EINO 适配层使用的工作流构建入口。
func (s *ExecutorService) BuildWorkflow(ctx context.Context, pipeline domain.Pipeline, task domain.Task) (WorkflowSpec, error) {
	if s == nil || s.workflowBuilder == nil {
		return WorkflowSpec{}, exception.NewServiceException("ingestion workflow builder is required", nil)
	}
	return s.workflowBuilder.Build(ctx, pipeline, task)
}

// RunnerForNode 返回某个节点类型对应的运行器。
func (s *ExecutorService) RunnerForNode(nodeType string) (NodeRunner, bool) {
	if s == nil || s.nodeRunners == nil {
		return nil, false
	}
	return s.nodeRunners.Get(strings.TrimSpace(nodeType))
}

// Observer 返回当前装配的 task observer。
func (s *ExecutorService) Observer() TaskObserver {
	if s == nil {
		return nil
	}
	return s.taskObserver
}

// newExecutionState 为后续 EINO workflow 执行准备共享上下文。
func (s *ExecutorService) newExecutionState(task domain.Task, pipeline domain.Pipeline) ExecutionState {
	return ExecutionState{
		Task:        task,
		Pipeline:    pipeline,
		NodeOutputs: map[string]map[string]any{},
		StartedAt:   s.now(),
	}
}

// runWorkflow 顺序执行最小 workflow 节点链路。
func (s *ExecutorService) runWorkflow(ctx context.Context, workflow WorkflowSpec, state ExecutionState) {
	if s == nil {
		return
	}
	task := state.Task
	startedAt := s.now()
	task.Status = domain.TaskStatusRunning
	task.StartedAt = &startedAt
	task.UpdatedAt = startedAt
	state.Task = task
	if s.taskObserver != nil {
		if err := s.taskObserver.OnTaskStarted(ctx, task); err != nil {
			return
		}
	}

	current := state
	var execErr error
	for _, item := range workflow.NodeOrder {
		runner, ok := s.RunnerForNode(item.Node.NodeType)
		if !ok {
			execErr = exception.NewClientException("node runner not found for node type: "+item.Node.NodeType, nil)
			break
		}

		if s.taskObserver != nil {
			if err := s.taskObserver.OnNodeStarted(ctx, task, item); err != nil {
				execErr = exception.NewServiceException("failed to mark ingestion node running", err)
				break
			}
		}

		startedAt := s.now()
		nextState, output, err := runner.Run(ctx, current, item.Node)
		duration := s.now().Sub(startedAt)
		if s.taskObserver != nil {
			if observeErr := s.taskObserver.OnNodeCompleted(ctx, task, item, output, duration, err); observeErr != nil && err == nil {
				err = exception.NewServiceException("failed to persist ingestion node result", observeErr)
			}
		}
		if err != nil {
			execErr = err
			break
		}
		if nextState.NodeOutputs == nil {
			nextState.NodeOutputs = current.NodeOutputs
		}
		if nextState.NodeOutputs == nil {
			nextState.NodeOutputs = map[string]map[string]any{}
		}
		if output != nil {
			nextState.NodeOutputs[item.Node.NodeID] = output
		}
		current = nextState
	}

	completedAt := s.now()
	task.CompletedAt = &completedAt
	task.UpdatedAt = completedAt
	current.CompletedAt = &completedAt
	current.Error = execErr
	current.Task = task
	if s.taskObserver != nil {
		_ = s.taskObserver.OnTaskCompleted(ctx, task, current, execErr)
	}
}
