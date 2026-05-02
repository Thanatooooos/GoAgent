package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
	"local/rag-project/internal/framework/distributedid"
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
		CreatedAt:  now,
		UpdatedAt:  now,
	})
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
	existing, err := o.taskNodeRepo.GetByTaskIDAndNodeID(ctx, task.ID, strings.TrimSpace(node.Node.NodeID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing.ID) == "" {
		if err := o.OnNodeStarted(ctx, task, node); err != nil {
			return err
		}
		existing, err = o.taskNodeRepo.GetByTaskIDAndNodeID(ctx, task.ID, strings.TrimSpace(node.Node.NodeID))
		if err != nil {
			return err
		}
	}
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
		record.ErrorMessage = execErr.Error()
	} else {
		record.Status = TaskNodeStatusSuccess
	}
	_, err = o.taskNodeRepo.Update(ctx, record)
	return err
}
