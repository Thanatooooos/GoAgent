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

// PipelineRepository 是 ingestion pipeline 的 postgres 实现。
type PipelineRepository struct {
	db *gorm.DB
}

func NewPipelineRepository(db *gorm.DB) *PipelineRepository {
	return &PipelineRepository{db: db}
}

func (r *PipelineRepository) Create(ctx context.Context, pipeline domain.Pipeline) (domain.Pipeline, error) {
	model, err := toPipelineModel(pipeline)
	if err != nil {
		return domain.Pipeline{}, err
	}
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Pipeline{}, fmt.Errorf("create ingestion pipeline: %w", err)
	}
	return toPipelineDomain(model)
}

func (r *PipelineRepository) Update(ctx context.Context, pipeline domain.Pipeline) (domain.Pipeline, error) {
	model, err := toPipelineModel(pipeline)
	if err != nil {
		return domain.Pipeline{}, err
	}
	result := r.db.WithContext(ctx).
		Model(&models.PipelineModel{}).
		Where("id = ?", pipeline.ID).
		Updates(map[string]any{
			"name":        model.Name,
			"description": model.Description,
			"nodes_json":  model.NodesJSON,
			"updated_by":  model.UpdatedBy,
			"update_time": model.UpdateTime,
		})
	if result.Error != nil {
		return domain.Pipeline{}, fmt.Errorf("update ingestion pipeline: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Pipeline{}, fmt.Errorf("update ingestion pipeline: no rows affected")
	}
	return pipeline, nil
}

func (r *PipelineRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.PipelineModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete ingestion pipeline: %w", err)
	}
	return nil
}

func (r *PipelineRepository) GetByID(ctx context.Context, id string) (domain.Pipeline, error) {
	var model models.PipelineModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Pipeline{}, nil
	}
	if err != nil {
		return domain.Pipeline{}, fmt.Errorf("get ingestion pipeline by id: %w", err)
	}
	item, err := toPipelineDomain(model)
	if err != nil {
		return domain.Pipeline{}, err
	}
	return item, nil
}

func (r *PipelineRepository) Count(ctx context.Context, filter port.PipelineListFilter) (int, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.PipelineModel{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count ingestion pipelines: %w", err)
	}
	return int(count), nil
}

func (r *PipelineRepository) List(ctx context.Context, filter port.PipelineListFilter) ([]domain.Pipeline, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.PipelineModel{}), filter).
		Order("create_time desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var items []models.PipelineModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list ingestion pipelines: %w", err)
	}
	return mustToPipelineDomains(items)
}

func (r *PipelineRepository) applyFilter(query *gorm.DB, filter port.PipelineListFilter) *gorm.DB {
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		query = query.Where("(name ILIKE ? OR description ILIKE ?)", like, like)
	}
	return query
}
