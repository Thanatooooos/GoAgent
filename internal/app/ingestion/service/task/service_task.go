package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

const (
	defaultTaskPage     = 1
	defaultTaskPageSize = 10
	maxTaskPageSize     = 100
)

// PageTasksInput 描述 task 分页查询参数。
type PageTasksInput struct {
	Page       int
	PageSize   int
	PipelineID string
	Status     string
}

// TaskPageResult 表示 task 分页结果。
type TaskPageResult struct {
	Items    []domain.Task
	Total    int
	Page     int
	PageSize int
}

// CreateTaskInput 描述创建 task 的入参。
type CreateTaskInput struct {
	ID             string
	PipelineID     string
	SourceType     string
	SourceLocation string
	SourceFileName string
	Metadata       map[string]any
	CreatedBy      string
}

// TaskService 负责 ingestion task 的创建与查询。
type TaskService struct {
	pipelineRepo port.PipelineRepository
	taskRepo     port.TaskRepository
	taskNodeRepo port.TaskNodeRepository
	executor     port.TaskExecutor
	now          func() time.Time
}

// NewTaskService 创建 task 服务。
func NewTaskService(
	pipelineRepo port.PipelineRepository,
	taskRepo port.TaskRepository,
	taskNodeRepo port.TaskNodeRepository,
	executor port.TaskExecutor,
) *TaskService {
	return &TaskService{
		pipelineRepo: pipelineRepo,
		taskRepo:     taskRepo,
		taskNodeRepo: taskNodeRepo,
		executor:     executor,
		now:          time.Now,
	}
}

// activeDocumentTaskChecker 用于判断文档是否已有活动中的 ingestion task。
type activeDocumentTaskChecker interface {
	HasActiveTaskForDocument(ctx context.Context, documentID string) (bool, error)
}

// Page 分页查询 task 列表。
func (s *TaskService) Page(ctx context.Context, input PageTasksInput) (TaskPageResult, error) {
	if s == nil || s.taskRepo == nil {
		return TaskPageResult{}, exception.NewServiceException("ingestion task repository is required", nil)
	}
	page := normalizeTaskPage(input.Page)
	pageSize := normalizeTaskPageSize(input.PageSize)
	filter := port.TaskListFilter{
		PipelineID: strings.TrimSpace(input.PipelineID),
		Status:     strings.TrimSpace(input.Status),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}

	total, err := s.taskRepo.Count(ctx, filter)
	if err != nil {
		return TaskPageResult{}, exception.NewServiceException("failed to count ingestion tasks", err)
	}
	items, err := s.taskRepo.List(ctx, filter)
	if err != nil {
		return TaskPageResult{}, exception.NewServiceException("failed to list ingestion tasks", err)
	}

	return TaskPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Get 查询单个 task。
func (s *TaskService) Get(ctx context.Context, id string) (domain.Task, error) {
	if s == nil || s.taskRepo == nil {
		return domain.Task{}, exception.NewServiceException("ingestion task repository is required", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Task{}, exception.NewClientException("task id is required", nil)
	}
	item, err := s.taskRepo.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, exception.NewServiceException("failed to load ingestion task", err)
	}
	if strings.TrimSpace(item.ID) == "" {
		return domain.Task{}, exception.NewClientException("ingestion task not found", nil)
	}
	return item, nil
}

// GetNode 查询指定 task 下的单个节点执行记录。
func (s *TaskService) GetNode(ctx context.Context, taskID string, nodeID string) (domain.TaskNode, error) {
	if s == nil || s.taskNodeRepo == nil {
		return domain.TaskNode{}, exception.NewServiceException("ingestion task node repository is required", nil)
	}
	taskID = strings.TrimSpace(taskID)
	nodeID = strings.TrimSpace(nodeID)
	if taskID == "" {
		return domain.TaskNode{}, exception.NewClientException("task id is required", nil)
	}
	if nodeID == "" {
		return domain.TaskNode{}, exception.NewClientException("node id is required", nil)
	}
	return s.taskNodeRepo.GetByTaskIDAndNodeID(ctx, taskID, nodeID)
}

// ListNodes 查询单个 task 下的节点执行记录。
func (s *TaskService) ListNodes(ctx context.Context, taskID string) ([]domain.TaskNode, error) {
	if s == nil || s.taskNodeRepo == nil {
		return nil, exception.NewServiceException("ingestion task node repository is required", nil)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, exception.NewClientException("task id is required", nil)
	}
	items, err := s.taskNodeRepo.ListByTaskID(ctx, taskID)
	if err != nil {
		return nil, exception.NewServiceException("failed to list ingestion task nodes", err)
	}
	return items, nil
}

// Create 创建一条 ingestion task，并在可用时提交给执行层。
func (s *TaskService) Create(ctx context.Context, input CreateTaskInput) (domain.Task, error) {
	if s == nil || s.pipelineRepo == nil || s.taskRepo == nil {
		return domain.Task{}, exception.NewServiceException("ingestion repositories are required", nil)
	}

	pipelineID := strings.TrimSpace(input.PipelineID)
	if pipelineID == "" {
		return domain.Task{}, exception.NewClientException("pipeline id is required", nil)
	}
	sourceType := strings.TrimSpace(input.SourceType)
	if err := ingestionworkflow.ValidateTaskSourceType(sourceType); err != nil {
		return domain.Task{}, err
	}

	pipeline, err := s.pipelineRepo.GetByID(ctx, pipelineID)
	if err != nil {
		return domain.Task{}, exception.NewServiceException("failed to load ingestion pipeline", err)
	}
	if strings.TrimSpace(pipeline.ID) == "" {
		return domain.Task{}, exception.NewClientException("ingestion pipeline not found", nil)
	}
	if documentID := ingestionworkflow.ReadStringSetting(input.Metadata, "documentId"); documentID != "" {
		if checker, ok := s.taskRepo.(activeDocumentTaskChecker); ok {
			hasActive, err := checker.HasActiveTaskForDocument(ctx, documentID)
			if err != nil {
				return domain.Task{}, exception.NewServiceException("failed to check active ingestion task for document", err)
			}
			if hasActive {
				return domain.Task{}, exception.NewClientException("ingestion task for document is already running", nil)
			}
		}
	}

	now := s.now()
	taskID := strings.TrimSpace(input.ID)
	if taskID == "" {
		id, err := distributedid.NextID()
		if err != nil {
			return domain.Task{}, exception.NewServiceException("failed to generate ingestion task id", err)
		}
		taskID = fmt.Sprintf("%d", id)
	}
	task := domain.Task{
		ID:             taskID,
		PipelineID:     pipelineID,
		SourceType:     sourceType,
		SourceLocation: strings.TrimSpace(input.SourceLocation),
		SourceFileName: strings.TrimSpace(input.SourceFileName),
		Status:         domain.TaskStatusPending,
		Metadata:       input.Metadata,
		CreatedBy:      strings.TrimSpace(input.CreatedBy),
		UpdatedBy:      strings.TrimSpace(input.CreatedBy),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	item, err := s.taskRepo.Create(ctx, task)
	if err != nil {
		return domain.Task{}, exception.NewServiceException("failed to create ingestion task", err)
	}

	// 第一阶段先保留执行提交边界；执行器存在时再真正异步推进。
	if s.executor != nil {
		if err := s.executor.Submit(ctx, pipeline, item); err != nil {
			return domain.Task{}, exception.NewServiceException("failed to submit ingestion task", err)
		}
	}
	return item, nil
}

// normalizeTaskPage 规范化分页页码。
func normalizeTaskPage(page int) int {
	if page <= 0 {
		return defaultTaskPage
	}
	return page
}

// normalizeTaskPageSize 规范化分页大小。
func normalizeTaskPageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultTaskPageSize
	}
	if pageSize > maxTaskPageSize {
		return maxTaskPageSize
	}
	return pageSize
}
