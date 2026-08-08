package dailybrief

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type GenerationRunRepository struct {
	db *gorm.DB
}

func NewGenerationRunRepository(db *gorm.DB) *GenerationRunRepository {
	return &GenerationRunRepository{db: db}
}

func (r *GenerationRunRepository) Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	model := toGenerationRunModel(run)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.GenerationRun{}, fmt.Errorf("create daily brief generation run: %w", err)
	}
	return toGenerationRunDomain(model), nil
}

func (r *GenerationRunRepository) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	model := toGenerationRunModel(run)
	result := r.db.WithContext(ctx).
		Model(&models.GenerationRunModel{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":            model.Status,
			"finished_at":       model.FinishedAt,
			"error_message":     model.ErrorMessage,
			"source_stats_json": model.SourceStatsJSON,
			"model":             model.Model,
			"prompt_version":    model.PromptVersion,
			"token_usage_json":  model.TokenUsageJSON,
			"update_time":       model.UpdateTime,
		})
	if result.Error != nil {
		return domain.GenerationRun{}, fmt.Errorf("update daily brief generation run: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.GenerationRun{}, fmt.Errorf("update daily brief generation run: no rows affected")
	}
	return run, nil
}

func (r *GenerationRunRepository) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	var model models.GenerationRunModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.GenerationRun{}, nil
	}
	if err != nil {
		return domain.GenerationRun{}, fmt.Errorf("get daily brief generation run by id: %w", err)
	}
	return toGenerationRunDomain(model), nil
}

func (r *GenerationRunRepository) GetLatestFailedByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.GenerationRun, error) {
	var model models.GenerationRunModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("brief_date = ?", briefDate).
		Where("status = ?", domain.GenerationRunStatusFailed).
		Where("finished_at IS NOT NULL").
		Order("finished_at desc").
		Limit(1).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.GenerationRun{}, nil
	}
	if err != nil {
		return domain.GenerationRun{}, fmt.Errorf("get latest failed daily brief generation run: %w", err)
	}
	return toGenerationRunDomain(model), nil
}

func (r *GenerationRunRepository) ListRetryEligible(ctx context.Context, filter port.GenerationRunRetryEligibleFilter) ([]domain.GenerationRun, error) {
	query := r.db.WithContext(ctx).
		Model(&models.GenerationRunModel{}).
		Where("status = ?", domain.GenerationRunStatusFailed).
		Where("finished_at IS NOT NULL").
		Where("finished_at <= ?", filter.FailedBefore).
		Order("finished_at asc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	var items []models.GenerationRunModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list daily brief retry eligible generation runs: %w", err)
	}
	result := make([]domain.GenerationRun, 0, len(items))
	for _, item := range items {
		result = append(result, toGenerationRunDomain(item))
	}
	return result, nil
}

func (r *GenerationRunRepository) CountRetryRunsByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.GenerationRunModel{}).
		Where("user_id = ?", userID).
		Where("brief_date = ?", briefDate).
		Where("trigger_type = ?", domain.GenerationRunTriggerTypeRetry).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count daily brief retry generation runs: %w", err)
	}
	return int(count), nil
}

var _ port.GenerationRunRepository = (*GenerationRunRepository)(nil)
