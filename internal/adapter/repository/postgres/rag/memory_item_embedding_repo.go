package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type MemoryItemEmbeddingRepository struct {
	db *gorm.DB
}

func NewMemoryItemEmbeddingRepository(db *gorm.DB) *MemoryItemEmbeddingRepository {
	return &MemoryItemEmbeddingRepository{db: db}
}

func (r *MemoryItemEmbeddingRepository) UpsertBatch(ctx context.Context, embeddings []domain.MemoryItemEmbedding) error {
	if len(embeddings) == 0 {
		return nil
	}

	now := time.Now()
	for _, item := range embeddings {
		if strings.TrimSpace(item.MemoryItemID) == "" {
			return fmt.Errorf("memory item embedding requires memory item id")
		}
		if len(item.Embedding) == 0 {
			return fmt.Errorf("memory item embedding is required: memoryItemID=%s", item.MemoryItemID)
		}
		createTime := item.CreateTime
		if createTime.IsZero() {
			createTime = now
		}
		updateTime := item.UpdateTime
		if updateTime.IsZero() {
			updateTime = now
		}
		if err := r.db.WithContext(ctx).Exec(`
	INSERT INTO t_memory_item_embedding
	    (memory_item_id, embedding, create_time, update_time)
	VALUES
	    (?, CAST(? AS vector), ?, ?)
	ON CONFLICT (memory_item_id) DO UPDATE SET
	    embedding = EXCLUDED.embedding,
	    update_time = EXCLUDED.update_time
	`,
			strings.TrimSpace(item.MemoryItemID),
			formatSessionVector(item.Embedding),
			createTime,
			updateTime,
		).Error; err != nil {
			return fmt.Errorf("upsert memory item embeddings: %w", err)
		}
	}
	return nil
}

func (r *MemoryItemEmbeddingRepository) SearchByVector(ctx context.Context, vector []float32, filter port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("gorm db is required")
	}
	if len(vector) == 0 {
		return []domain.MemoryItemSearchHit{}, nil
	}
	topK := filter.TopK
	if topK <= 0 {
		topK = 8
	}

	vectorLiteral := formatSessionVector(vector)
	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString(`
SELECT
	mi.id,
	mi.user_id,
	mi.scope_type,
	mi.scope_id,
	mi.namespace,
	mi.memory_type,
	mi.category,
	mi.canonical_key,
	mi.value_type,
	mi.value_json,
	mi.display_value,
	mi.source_message_id,
	mi.content,
	mi.summary,
	mi.confidence,
	mi.importance,
	mi.status,
	mi.last_confirmed_at,
	mi.last_used_at,
	mi.expires_at,
	mi.supersedes_id,
	mi.extraction_method,
	mi.created_by,
	mi.updated_by,
	mi.create_time,
	mi.update_time,
	1 - (mie.embedding <=> CAST(? AS vector)) AS score
FROM t_memory_item AS mi
INNER JOIN t_memory_item_embedding AS mie ON mie.memory_item_id = mi.id
WHERE mi.deleted = 0
  AND mi.user_id = ?
`)
	args := []any{vectorLiteral, strings.TrimSpace(filter.UserID)}
	if values := trimNonEmpty(filter.ScopeTypes); len(values) > 0 {
		sqlBuilder.WriteString("  AND mi.scope_type IN ?\n")
		args = append(args, values)
	}
	if values := trimNonEmpty(filter.ScopeIDs); len(values) > 0 {
		sqlBuilder.WriteString("  AND mi.scope_id IN ?\n")
		args = append(args, values)
	}
	if values := trimNonEmpty(filter.Statuses); len(values) > 0 {
		sqlBuilder.WriteString("  AND mi.status IN ?\n")
		args = append(args, values)
	}
	sqlBuilder.WriteString("ORDER BY mie.embedding <=> CAST(? AS vector)\nLIMIT ?")
	args = append(args, vectorLiteral, topK)

	rows, err := r.db.WithContext(ctx).Raw(sqlBuilder.String(), args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("search memory item embeddings: %w", err)
	}
	defer rows.Close()

	result := make([]domain.MemoryItemSearchHit, 0, topK)
	for rows.Next() {
		var hit domain.MemoryItemSearchHit
		if err := rows.Scan(
			&hit.ID,
			&hit.UserID,
			&hit.ScopeType,
			&hit.ScopeID,
			&hit.Namespace,
			&hit.MemoryType,
			&hit.Category,
			&hit.CanonicalKey,
			&hit.ValueType,
			&hit.ValueJSON,
			&hit.DisplayValue,
			&hit.SourceMessageID,
			&hit.Content,
			&hit.Summary,
			&hit.Confidence,
			&hit.Importance,
			&hit.Status,
			&hit.LastConfirmedAt,
			&hit.LastUsedAt,
			&hit.ExpiresAt,
			&hit.SupersedesID,
			&hit.ExtractionMethod,
			&hit.CreatedBy,
			&hit.UpdatedBy,
			&hit.CreateTime,
			&hit.UpdateTime,
			&hit.Score,
		); err != nil {
			return nil, fmt.Errorf("scan memory item search hit: %w", err)
		}
		result = append(result, hit)
	}
	return result, nil
}
