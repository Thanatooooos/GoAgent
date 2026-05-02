package knowledge

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeDocumentChunkLogRepository struct {
	db *gorm.DB
}

func NewKnowledgeDocumentChunkLogRepository(db *gorm.DB) *KnowledgeDocumentChunkLogRepository {
	return &KnowledgeDocumentChunkLogRepository{db: db}
}

func (r *KnowledgeDocumentChunkLogRepository) Create(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
	model := toKnowledgeDocumentChunkLogModel(log)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeDocumentChunkLog{}, fmt.Errorf("create knowledge document chunk log: %w", err)
	}
	return toKnowledgeDocumentChunkLogDomain(model), nil
}

func (r *KnowledgeDocumentChunkLogRepository) Update(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
	updates := map[string]any{
		"status":           log.Status,
		"process_mode":     log.ProcessMode,
		"chunk_strategy":   log.ChunkStrategy,
		"pipeline_id":      log.PipelineID,
		"extract_duration": log.ExtractDuration,
		"chunk_duration":   log.ChunkDuration,
		"embed_duration":   log.EmbedDuration,
		"persist_duration": log.PersistDuration,
		"total_duration":   log.TotalDuration,
		"chunk_count":      log.ChunkCount,
		"error_message":    log.ErrorMessage,
		"start_time":       log.StartTime,
		"end_time":         log.EndTime,
		"update_time":      log.UpdatedAt,
	}

	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentChunkLogModel{}).
		Where("id = ?", log.ID).
		Updates(updates)
	if result.Error != nil {
		return domain.KnowledgeDocumentChunkLog{}, fmt.Errorf("update knowledge document chunk log: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.KnowledgeDocumentChunkLog{}, fmt.Errorf("update knowledge document chunk log: no rows affected")
	}
	return log, nil
}

func (r *KnowledgeDocumentChunkLogRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentChunkLog, error) {
	var model models.KnowledgeDocumentChunkLogModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeDocumentChunkLog{}, nil
	}
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, fmt.Errorf("get knowledge document chunk log by id: %w", err)
	}
	return toKnowledgeDocumentChunkLogDomain(model), nil
}

func (r *KnowledgeDocumentChunkLogRepository) GetByTaskID(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
	return r.GetByID(ctx, taskID)
}

func (r *KnowledgeDocumentChunkLogRepository) CountByDocumentID(ctx context.Context, documentID string) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentChunkLogModel{}).
		Where("doc_id = ?", documentID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count knowledge document chunk logs: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeDocumentChunkLogRepository) ListByDocumentID(ctx context.Context, documentID string, options port.ListOptions) ([]domain.KnowledgeDocumentChunkLog, error) {
	query := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentChunkLogModel{}).
		Where("doc_id = ?", documentID).
		Order("create_time desc")

	if options.Limit > 0 {
		query = query.Limit(options.Limit)
	}
	if options.Offset > 0 {
		query = query.Offset(options.Offset)
	}

	var items []models.KnowledgeDocumentChunkLogModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list knowledge document chunk logs: %w", err)
	}

	result := make([]domain.KnowledgeDocumentChunkLog, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeDocumentChunkLogDomain(item))
	}
	return result, nil
}
