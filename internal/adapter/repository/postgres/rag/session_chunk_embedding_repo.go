package rag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/domain"
)

type SessionChunkEmbeddingRepository struct {
	db *gorm.DB
}

func NewSessionChunkEmbeddingRepository(db *gorm.DB) *SessionChunkEmbeddingRepository {
	return &SessionChunkEmbeddingRepository{db: db}
}

func (r *SessionChunkEmbeddingRepository) UpsertBatch(ctx context.Context, embeddings []domain.SessionChunkEmbedding) error {
	if len(embeddings) == 0 {
		return nil
	}

	now := time.Now()
	for _, item := range embeddings {
		if len(item.Embedding) == 0 {
			return fmt.Errorf("session chunk embedding is required: chunkID=%s", item.ChunkID)
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
	INSERT INTO t_session_chunk_embedding
	    (chunk_id, embedding, create_time, update_time)
	VALUES
	    (?, CAST(? AS vector), ?, ?)
	ON CONFLICT (chunk_id) DO UPDATE SET
	    embedding = EXCLUDED.embedding,
	    update_time = EXCLUDED.update_time
	`,
			strings.TrimSpace(item.ChunkID),
			formatSessionVector(item.Embedding),
			createTime,
			updateTime,
		).Error; err != nil {
			return fmt.Errorf("upsert session chunk embeddings: %w", err)
		}
	}
	return nil
}

func formatSessionVector(values []float32) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
