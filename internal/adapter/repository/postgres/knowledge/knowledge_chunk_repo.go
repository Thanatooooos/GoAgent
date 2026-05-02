package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeChunkRepository struct {
	db *gorm.DB
}

func NewKnowledgeChunkRepository(db *gorm.DB) *KnowledgeChunkRepository {
	return &KnowledgeChunkRepository{db: db}
}

func (r *KnowledgeChunkRepository) Create(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	model := toKnowledgeChunkModel(chunk)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeChunk{}, fmt.Errorf("create knowledge chunk: %w", err)
	}
	return toKnowledgeChunkDomain(model), nil
}

func (r *KnowledgeChunkRepository) CreateBatch(ctx context.Context, chunks []domain.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	items := make([]models.KnowledgeChunkModel, 0, len(chunks))
	for _, chunk := range chunks {
		items = append(items, toKnowledgeChunkModel(chunk))
	}

	if err := r.db.WithContext(ctx).Create(&items).Error; err != nil {
		return fmt.Errorf("create knowledge chunks: %w", err)
	}
	return nil
}

func (r *KnowledgeChunkRepository) Update(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	updates := map[string]any{
		"kb_id":        chunk.KnowledgeBaseID,
		"doc_id":       chunk.DocumentID,
		"chunk_index":  chunk.ChunkIndex,
		"content":      chunk.Content,
		"content_hash": chunk.ContentHash,
		"char_count":   chunk.CharCount,
		"token_count":  chunk.TokenCount,
		"enabled":      boolToFlag(chunk.Enabled),
		"updated_by":   chunk.UpdatedBy,
		"update_time":  chunk.UpdatedAt,
	}

	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeChunkModel{}).
		Where("id = ?", chunk.ID).
		Updates(updates)
	if result.Error != nil {
		return domain.KnowledgeChunk{}, fmt.Errorf("update knowledge chunk: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.KnowledgeChunk{}, fmt.Errorf("update knowledge chunk: no rows affected")
	}
	return chunk, nil
}

func (r *KnowledgeChunkRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeChunkModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete knowledge chunk: %w", err)
	}
	return nil
}

func (r *KnowledgeChunkRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeChunkModel{}, "doc_id = ?", documentID).Error; err != nil {
		return fmt.Errorf("delete knowledge chunks by document id: %w", err)
	}
	return nil
}

func (r *KnowledgeChunkRepository) UpdateEnabledByDocumentID(ctx context.Context, documentID string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeChunkModel{}).
		Where("doc_id = ?", documentID).
		Updates(map[string]any{
			"enabled":     boolToFlag(enabled),
			"updated_by":  updatedBy,
			"update_time": updatedAt,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge chunk enabled by document id: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeChunkRepository) UpdateEnabledByIDs(ctx context.Context, documentID string, chunkIDs []string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	if len(chunkIDs) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeChunkModel{}).
		Where("doc_id = ?", documentID).
		Where("id IN ?", chunkIDs).
		Updates(map[string]any{
			"enabled":     boolToFlag(enabled),
			"updated_by":  updatedBy,
			"update_time": updatedAt,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge chunk enabled by ids: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeChunkRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeChunk, error) {
	var model models.KnowledgeChunkModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeChunk{}, nil
	}
	if err != nil {
		return domain.KnowledgeChunk{}, fmt.Errorf("get knowledge chunk by id: %w", err)
	}
	return toKnowledgeChunkDomain(model), nil
}

func (r *KnowledgeChunkRepository) CountByDocumentID(ctx context.Context, documentID string, enabled *bool) (int, error) {
	query := r.db.WithContext(ctx).Model(&models.KnowledgeChunkModel{}).Where("doc_id = ?", documentID)
	if enabled != nil {
		query = query.Where("enabled = ?", boolToFlag(*enabled))
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count knowledge chunks by document id: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeChunkRepository) List(ctx context.Context, filter port.KnowledgeChunkListFilter) ([]domain.KnowledgeChunk, error) {
	query := r.applyKnowledgeChunkListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeChunkModel{}), filter).
		Order("chunk_index asc").
		Order("create_time asc")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.KnowledgeChunkModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list knowledge chunks: %w", err)
	}

	result := make([]domain.KnowledgeChunk, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeChunkDomain(item))
	}
	return result, nil
}

func (r *KnowledgeChunkRepository) applyKnowledgeChunkListFilter(query *gorm.DB, filter port.KnowledgeChunkListFilter) *gorm.DB {
	if filter.DocumentID != "" {
		query = query.Where("doc_id = ?", filter.DocumentID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", boolToFlag(*filter.Enabled))
	}
	return query
}
