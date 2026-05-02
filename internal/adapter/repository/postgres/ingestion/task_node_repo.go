package ingestion

import (
	"context"
	"errors"
	"fmt"

	"local/rag-project/internal/adapter/repository/postgres/ingestion/models"
	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"

	"gorm.io/gorm"
)

// TaskNodeRepository 是 ingestion task node 的 postgres 实现。
type TaskNodeRepository struct {
	db *gorm.DB
}

func NewTaskNodeRepository(db *gorm.DB) *TaskNodeRepository {
	return &TaskNodeRepository{db: db}
}

func (r *TaskNodeRepository) Create(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error) {
	model, err := toTaskNodeModel(node)
	if err != nil {
		return domain.TaskNode{}, err
	}
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.TaskNode{}, fmt.Errorf("create ingestion task node: %w", err)
	}
	return toTaskNodeDomain(model)
}

func (r *TaskNodeRepository) Update(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error) {
	model, err := toTaskNodeModel(node)
	if err != nil {
		return domain.TaskNode{}, err
	}
	result := r.db.WithContext(ctx).
		Model(&models.TaskNodeModel{}).
		Where("id = ?", node.ID).
		Updates(map[string]any{
			"task_id":       model.TaskID,
			"pipeline_id":   model.PipelineID,
			"node_id":       model.NodeID,
			"node_type":     model.NodeType,
			"node_order":    model.NodeOrder,
			"status":        model.Status,
			"duration_ms":   model.DurationMs,
			"message":       model.Message,
			"error_message": model.ErrorMessage,
			"output":        model.Output,
			"update_time":   model.UpdateTime,
		})
	if result.Error != nil {
		return domain.TaskNode{}, fmt.Errorf("update ingestion task node: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.TaskNode{}, fmt.Errorf("update ingestion task node: no rows affected")
	}
	return node, nil
}

func (r *TaskNodeRepository) GetByTaskIDAndNodeID(ctx context.Context, taskID string, nodeID string) (domain.TaskNode, error) {
	var model models.TaskNodeModel
	err := r.db.WithContext(ctx).
		Where("task_id = ? AND node_id = ?", taskID, nodeID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TaskNode{}, nil
	}
	if err != nil {
		return domain.TaskNode{}, fmt.Errorf("get ingestion task node by task and node id: %w", err)
	}
	return toTaskNodeDomain(model)
}

func (r *TaskNodeRepository) ListByTaskID(ctx context.Context, taskID string) ([]domain.TaskNode, error) {
	query := r.db.WithContext(ctx).
		Model(&models.TaskNodeModel{}).
		Where("task_id = ?", taskID).
		Order("node_order asc").
		Order("create_time asc")
	var items []models.TaskNodeModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list ingestion task nodes by task id: %w", err)
	}
	return mustToTaskNodeDomains(items)
}

var (
	_ port.PipelineRepository = (*PipelineRepository)(nil)
	_ port.TaskRepository     = (*TaskRepository)(nil)
	_ port.TaskNodeRepository = (*TaskNodeRepository)(nil)
)
