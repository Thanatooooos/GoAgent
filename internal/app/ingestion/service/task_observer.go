package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

const (
	// TaskNodeStatusPending 表示节点尚未开始。
	TaskNodeStatusPending = "pending"
	// TaskNodeStatusRunning 表示节点执行中。
	TaskNodeStatusRunning = "running"
	// TaskNodeStatusSuccess 表示节点执行成功。
	TaskNodeStatusSuccess = "success"
	// TaskNodeStatusFailed 表示节点执行失败。
	TaskNodeStatusFailed = "failed"
)

// TaskObserver 定义 ingestion 执行过程中的任务观测边界。
type TaskObserver interface {
	OnTaskStarted(ctx context.Context, task domain.Task) error
	OnTaskCompleted(ctx context.Context, task domain.Task, state ExecutionState, execErr error) error
	OnNodeStarted(ctx context.Context, task domain.Task, node WorkflowNodeSpec) error
	OnNodeRetry(ctx context.Context, task domain.Task, node WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error
	OnNodeCompleted(ctx context.Context, task domain.Task, node WorkflowNodeSpec, output map[string]any, duration time.Duration, execErr error) error
}

// RepositoryTaskObserver 使用 task/task_node repository 回写执行观测数据。
type RepositoryTaskObserver struct {
	taskRepo     port.TaskRepository
	taskNodeRepo port.TaskNodeRepository
	now          func() time.Time
}

// NewRepositoryTaskObserver 创建 repository 驱动的 task observer。
func NewRepositoryTaskObserver(
	taskRepo port.TaskRepository,
	taskNodeRepo port.TaskNodeRepository,
) *RepositoryTaskObserver {
	return &RepositoryTaskObserver{
		taskRepo:     taskRepo,
		taskNodeRepo: taskNodeRepo,
		now:          time.Now,
	}
}

// OnTaskStarted 回写 task 开始状态。
func (o *RepositoryTaskObserver) OnTaskStarted(ctx context.Context, task domain.Task) error {
	if o == nil || o.taskRepo == nil {
		return nil
	}
	now := o.now()
	task.Status = domain.TaskStatusRunning
	task.StartedAt = &now
	task.UpdatedAt = now
	_, err := o.taskRepo.Update(ctx, task)
	return err
}

// OnTaskCompleted 回写 task 完成状态。
func (o *RepositoryTaskObserver) OnTaskCompleted(ctx context.Context, task domain.Task, state ExecutionState, execErr error) error {
	if o == nil || o.taskRepo == nil {
		return nil
	}
	now := o.now()
	task.UpdatedAt = now
	task.CompletedAt = &now
	task.ChunkCount = len(state.Chunks)
	if execErr != nil {
		task.Status = domain.TaskStatusFailed
		task.ErrorMessage = execErr.Error()
	} else {
		task.Status = domain.TaskStatusSuccess
		task.ErrorMessage = ""
	}
	_, err := o.taskRepo.Update(ctx, task)
	return err
}

// OnNodeStarted 写入或更新节点开始状态。
func (o *RepositoryTaskObserver) OnNodeStarted(ctx context.Context, task domain.Task, node WorkflowNodeSpec) error {
	if o == nil || o.taskNodeRepo == nil {
		return nil
	}
	now := o.now()
	existing, err := o.taskNodeRepo.GetByTaskIDAndNodeID(ctx, task.ID, strings.TrimSpace(node.Node.NodeID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing.ID) != "" {
		existing.Status = TaskNodeStatusRunning
		existing.NodeOrder = node.Order
		existing.NodeType = strings.TrimSpace(node.Node.NodeType)
		existing.Message = "node running"
		existing.ErrorMessage = ""
		existing.UpdatedAt = now
		_, err = o.taskNodeRepo.Update(ctx, existing)
		return err
	}
	id, err := distributedid.NextID()
	if err != nil {
		return fmt.Errorf("generate ingestion task node id: %w", err)
	}
	_, err = o.taskNodeRepo.Create(ctx, domain.TaskNode{
		ID:         fmt.Sprintf("%d", id),
		TaskID:     task.ID,
		PipelineID: task.PipelineID,
		NodeID:     strings.TrimSpace(node.Node.NodeID),
		NodeType:   strings.TrimSpace(node.Node.NodeType),
		NodeOrder:  node.Order,
		Status:     TaskNodeStatusRunning,
		Message:    "node running",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return err
}

// OnNodeRetry 持久化节点重试信息，便于后续排障。
func (o *RepositoryTaskObserver) OnNodeRetry(ctx context.Context, task domain.Task, node WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error {
	if o == nil || o.taskNodeRepo == nil {
		return nil
	}
	existing, err := o.ensureTaskNodeRecord(ctx, task, node)
	if err != nil {
		return err
	}

	now := o.now()
	output := cloneTaskNodeOutput(existing.Output)
	retryCount := readIntSetting(output, "retryCount")
	if attempt > retryCount {
		retryCount = attempt
	}
	output["retryCount"] = retryCount
	output["lastRetryAttempt"] = attempt
	output["lastRetryBackoffMs"] = backoff.Milliseconds()
	output["lastRetryAt"] = now.Format(time.RFC3339Nano)
	if execErr != nil {
		output["lastError"] = execErr.Error()
		output["errorCategory"] = classifyTaskNodeError(execErr)
	}

	existing.Status = TaskNodeStatusRunning
	existing.Message = truncateTaskNodeText(fmt.Sprintf("retrying attempt %d after error", attempt), 1000)
	existing.ErrorMessage = ""
	existing.Output = output
	existing.UpdatedAt = now
	_, err = o.taskNodeRepo.Update(ctx, existing)
	return err
}

// OnNodeCompleted 回写节点执行结果。
func (o *RepositoryTaskObserver) OnNodeCompleted(
	ctx context.Context,
	task domain.Task,
	node WorkflowNodeSpec,
	output map[string]any,
	duration time.Duration,
	execErr error,
) error {
	if o == nil || o.taskNodeRepo == nil {
		return nil
	}
	now := o.now()
	existing, err := o.ensureTaskNodeRecord(ctx, task, node)
	if err != nil {
		return err
	}
	output = buildObservedTaskNodeOutput(existing.Output, output, duration, execErr)
	record := domain.TaskNode{
		ID:         existing.ID,
		TaskID:     task.ID,
		PipelineID: task.PipelineID,
		NodeID:     strings.TrimSpace(node.Node.NodeID),
		NodeType:   strings.TrimSpace(node.Node.NodeType),
		NodeOrder:  node.Order,
		DurationMs: duration.Milliseconds(),
		Output:     output,
		UpdatedAt:  now,
	}
	if execErr != nil {
		record.Status = TaskNodeStatusFailed
		record.Message = truncateTaskNodeText("node failed", 1000)
		record.ErrorMessage = execErr.Error()
	} else {
		record.Status = TaskNodeStatusSuccess
		record.Message = truncateTaskNodeText("node completed", 1000)
		record.ErrorMessage = ""
	}
	_, err = o.taskNodeRepo.Update(ctx, record)
	return err
}

// ensureTaskNodeRecord 确保节点记录存在，避免重试事件丢失。
func (o *RepositoryTaskObserver) ensureTaskNodeRecord(ctx context.Context, task domain.Task, node WorkflowNodeSpec) (domain.TaskNode, error) {
	existing, err := o.taskNodeRepo.GetByTaskIDAndNodeID(ctx, task.ID, strings.TrimSpace(node.Node.NodeID))
	if err != nil {
		return domain.TaskNode{}, err
	}
	if strings.TrimSpace(existing.ID) != "" {
		return existing, nil
	}
	if err := o.OnNodeStarted(ctx, task, node); err != nil {
		return domain.TaskNode{}, err
	}
	return o.taskNodeRepo.GetByTaskIDAndNodeID(ctx, task.ID, strings.TrimSpace(node.Node.NodeID))
}

// buildObservedTaskNodeOutput 合并业务输出与执行观测元数据。
func buildObservedTaskNodeOutput(existing map[string]any, latest map[string]any, duration time.Duration, execErr error) map[string]any {
	result := cloneTaskNodeOutput(existing)
	for key, value := range latest {
		result[key] = value
	}
	retryCount := readIntSetting(result, "retryCount")
	result["retryCount"] = retryCount
	result["attemptCount"] = retryCount + 1
	result["durationMs"] = duration.Milliseconds()
	result["success"] = execErr == nil
	if execErr != nil {
		result["lastError"] = execErr.Error()
		result["errorCategory"] = classifyTaskNodeError(execErr)
	}
	return result
}

// cloneTaskNodeOutput 复制 task node output，避免复用底层 map。
func cloneTaskNodeOutput(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// classifyTaskNodeError 给节点错误打粗粒度分类，便于后续做定向处理。
func classifyTaskNodeError(err error) string {
	if err == nil {
		return ""
	}
	var clientErr *exception.ClientException
	if errors.As(err, &clientErr) {
		return "client"
	}
	var serviceErr *exception.ServiceException
	if errors.As(err, &serviceErr) {
		return "service"
	}
	return "unknown"
}

// truncateTaskNodeText 控制 message/error 文本长度，避免持久化字段溢出。
func truncateTaskNodeText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
