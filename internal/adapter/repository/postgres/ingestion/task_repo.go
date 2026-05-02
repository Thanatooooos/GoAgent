package ingestion

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/ingestion/models"
	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
)

// TaskRepository 是 ingestion task 的 postgres 实现。
type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	model, err := toTaskModel(task)
	if err != nil {
		return domain.Task{}, err
	}
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Task{}, fmt.Errorf("create ingestion task: %w", err)
	}
	return toTaskDomain(model)
}

func (r *TaskRepository) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	model, err := toTaskModel(task)
	if err != nil {
		return domain.Task{}, err
	}
	result := r.db.WithContext(ctx).
		Model(&models.TaskModel{}).
		Where("id = ?", task.ID).
		Updates(map[string]any{
			"pipeline_id":      model.PipelineID,
			"source_type":      model.SourceType,
			"source_location":  model.SourceLocation,
			"source_file_name": model.SourceFileName,
			"status":           model.Status,
			"chunk_count":      model.ChunkCount,
			"error_message":    model.ErrorMessage,
			"metadata":         model.Metadata,
			"started_at":       model.StartedAt,
			"completed_at":     model.CompletedAt,
			"updated_by":       model.UpdatedBy,
			"update_time":      model.UpdateTime,
		})
	if result.Error != nil {
		return domain.Task{}, fmt.Errorf("update ingestion task: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Task{}, fmt.Errorf("update ingestion task: no rows affected")
	}
	return task, nil
}

func (r *TaskRepository) GetByID(ctx context.Context, id string) (domain.Task, error) {
	var model models.TaskModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, nil
	}
	if err != nil {
		return domain.Task{}, fmt.Errorf("get ingestion task by id: %w", err)
	}
	return toTaskDomain(model)
}

func (r *TaskRepository) Count(ctx context.Context, filter port.TaskListFilter) (int, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.TaskModel{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count ingestion tasks: %w", err)
	}
	return int(count), nil
}

func (r *TaskRepository) List(ctx context.Context, filter port.TaskListFilter) ([]domain.Task, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.TaskModel{}), filter).
		Order("create_time desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var items []models.TaskModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list ingestion tasks: %w", err)
	}
	return mustToTaskDomains(items)
}

func (r *TaskRepository) applyFilter(query *gorm.DB, filter port.TaskListFilter) *gorm.DB {
	if filter.PipelineID != "" {
		query = query.Where("pipeline_id = ?", filter.PipelineID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	return query
}
