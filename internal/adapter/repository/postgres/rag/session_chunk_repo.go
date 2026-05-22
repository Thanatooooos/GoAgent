package rag

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
)

type SessionChunkRepository struct {
	db *gorm.DB
}

func NewSessionChunkRepository(db *gorm.DB) *SessionChunkRepository {
	return &SessionChunkRepository{db: db}
}

func (r *SessionChunkRepository) CreateBatch(ctx context.Context, chunks []domain.SessionChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	modelsBatch := make([]models.SessionChunkModel, 0, len(chunks))
	for _, chunk := range chunks {
		modelsBatch = append(modelsBatch, toSessionChunkModel(chunk))
	}
	if err := r.db.WithContext(ctx).Create(&modelsBatch).Error; err != nil {
		return fmt.Errorf("create session chunks: %w", err)
	}
	return nil
}

func (r *SessionChunkRepository) ExistsRecallable(ctx context.Context, conversationID string, userID string, excludeMessageID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("gorm db is required")
	}

	query := strings.TrimSpace(`
SELECT EXISTS (
	SELECT 1
	FROM t_session_chunk AS sc
	INNER JOIN t_message AS m ON m.id = sc.message_id
	WHERE sc.deleted = 0
	  AND m.deleted = 0
	  AND sc.conversation_id = ?
	  AND sc.user_id = ?
	  AND m.role = 'user'
	  AND m.is_summarized = true
`)
	args := []any{strings.TrimSpace(conversationID), strings.TrimSpace(userID)}
	if trimmed := strings.TrimSpace(excludeMessageID); trimmed != "" {
		query += "\n  AND sc.message_id <> ?"
		args = append(args, trimmed)
	}
	query += "\n)"

	var exists bool
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("check recallable session chunks: %w", err)
	}
	return exists, nil
}

func (r *SessionChunkRepository) SearchRecallableByVector(ctx context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("gorm db is required")
	}
	if len(vector) == 0 {
		return []domain.SessionChunkSearchHit{}, nil
	}
	if topK <= 0 {
		topK = 6
	}

	vectorLiteral := formatSessionVector(vector)
	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString(`
SELECT
	sc.id,
	sc.conversation_id,
	sc.message_id,
	sc.user_id,
	sc.chunk_index,
	sc.content,
	sc.content_summary,
	sc.token_estimate,
	sc.create_time,
	sc.update_time,
	1 - (sce.embedding <=> CAST(? AS vector)) AS score
FROM t_session_chunk AS sc
INNER JOIN t_session_chunk_embedding AS sce ON sce.chunk_id = sc.id
INNER JOIN t_message AS m ON m.id = sc.message_id
WHERE sc.deleted = 0
  AND m.deleted = 0
  AND sc.conversation_id = ?
  AND sc.user_id = ?
  AND m.role = 'user'
  AND m.is_summarized = true
`)
	args := []any{vectorLiteral, strings.TrimSpace(conversationID), strings.TrimSpace(userID)}
	if trimmed := strings.TrimSpace(excludeMessageID); trimmed != "" {
		sqlBuilder.WriteString("  AND sc.message_id <> ?\n")
		args = append(args, trimmed)
	}
	sqlBuilder.WriteString("ORDER BY sce.embedding <=> CAST(? AS vector)\nLIMIT ?")
	args = append(args, vectorLiteral, topK)

	rows, err := r.db.WithContext(ctx).Raw(sqlBuilder.String(), args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("search recallable session chunks: %w", err)
	}
	defer rows.Close()

	result := make([]domain.SessionChunkSearchHit, 0, topK)
	for rows.Next() {
		var hit domain.SessionChunkSearchHit
		if err := rows.Scan(
			&hit.ID,
			&hit.ConversationID,
			&hit.MessageID,
			&hit.UserID,
			&hit.ChunkIndex,
			&hit.Content,
			&hit.ContentSummary,
			&hit.TokenEstimate,
			&hit.CreateTime,
			&hit.UpdateTime,
			&hit.Score,
		); err != nil {
			return nil, fmt.Errorf("scan recallable session chunk: %w", err)
		}
		result = append(result, hit)
	}
	return result, nil
}
