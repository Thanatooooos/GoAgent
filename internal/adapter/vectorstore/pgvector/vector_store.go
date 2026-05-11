package pgvector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/knowledge/port"
	corevector "local/rag-project/internal/app/rag/core/vector"
)

type VectorStore struct {
	db *gorm.DB
}

var _ port.VectorStore = (*VectorStore)(nil)
var _ corevector.Searcher = (*VectorStore)(nil)

func NewVectorStore(db *gorm.DB) *VectorStore {
	return &VectorStore{db: db}
}

func (s *VectorStore) UpsertDocumentChunks(ctx context.Context, chunks []port.ChunkVector) error {
	if len(chunks) == 0 {
		return nil
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres vector store db is required")
	}

	now := time.Now()
	for _, chunk := range chunks {
		if len(chunk.Embedding) == 0 {
			return fmt.Errorf("chunk embedding is required: chunkID=%s", chunk.ChunkID)
		}
		metadata, err := json.Marshal(normalizeMetadata(chunk.Metadata))
		if err != nil {
			return fmt.Errorf("marshal vector metadata: %w", err)
		}
		if err := s.db.WithContext(ctx).Exec(`
INSERT INTO t_knowledge_chunk_vector
    (chunk_id, doc_id, kb_id, chunk_index, content, embedding, metadata, create_time, update_time)
VALUES
    (?, ?, ?, ?, ?, CAST(? AS vector), CAST(? AS jsonb), ?, ?)
ON CONFLICT (chunk_id) DO UPDATE SET
    doc_id = EXCLUDED.doc_id,
    kb_id = EXCLUDED.kb_id,
    chunk_index = EXCLUDED.chunk_index,
    content = EXCLUDED.content,
    embedding = EXCLUDED.embedding,
    metadata = EXCLUDED.metadata,
    update_time = EXCLUDED.update_time
`,
			chunk.ChunkID,
			chunk.DocumentID,
			chunk.KnowledgeBaseID,
			chunk.Index,
			chunk.Text,
			formatVector(chunk.Embedding),
			string(metadata),
			now,
			now,
		).Error; err != nil {
			return fmt.Errorf("upsert document chunk vector: %w", err)
		}
	}
	return nil
}

func (s *VectorStore) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres vector store db is required")
	}
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return nil
	}
	if err := s.db.WithContext(ctx).
		Exec("DELETE FROM t_knowledge_chunk_vector WHERE doc_id = ?", documentID).
		Error; err != nil {
		return fmt.Errorf("delete document vectors: %w", err)
	}
	return nil
}

func (s *VectorStore) DeleteChunk(ctx context.Context, chunkID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres vector store db is required")
	}
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return nil
	}
	if err := s.db.WithContext(ctx).
		Exec("DELETE FROM t_knowledge_chunk_vector WHERE chunk_id = ?", chunkID).
		Error; err != nil {
		return fmt.Errorf("delete chunk vector: %w", err)
	}
	return nil
}

func (s *VectorStore) DeleteChunks(ctx context.Context, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres vector store db is required")
	}
	trimmed := make([]string, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		chunkID = strings.TrimSpace(chunkID)
		if chunkID != "" {
			trimmed = append(trimmed, chunkID)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).
		Exec("DELETE FROM t_knowledge_chunk_vector WHERE chunk_id IN ?", trimmed).
		Error; err != nil {
		return fmt.Errorf("delete chunk vectors: %w", err)
	}
	return nil
}

// SearchByKeyword 使用 pg_trgm 的 word_similarity 做中文关键词模糊检索。
func (s *VectorStore) SearchByKeyword(ctx context.Context, query string, knowledgeBaseIDs []string, topK int) ([]corevector.SearchHit, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres vector store db is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []corevector.SearchHit{}, nil
	}
	if topK <= 0 {
		topK = 5
	}

	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString(`
SELECT chunk_id, doc_id, kb_id, chunk_index, content, metadata, word_similarity(content, ?) AS score
FROM t_knowledge_chunk_vector
WHERE word_similarity(content, ?) > 0
`)
	args := []any{query, query}
	if len(knowledgeBaseIDs) > 0 {
		sqlBuilder.WriteString("AND kb_id IN ?\n")
		args = append(args, knowledgeBaseIDs)
	}
	sqlBuilder.WriteString("ORDER BY score DESC\nLIMIT ?")
	args = append(args, topK)

	queryResult := s.db.WithContext(ctx).Raw(sqlBuilder.String(), args...)

	rows, err := queryResult.Rows()
	if err != nil {
		return nil, fmt.Errorf("keyword search chunks: %w", err)
	}
	defer rows.Close()

	result := make([]corevector.SearchHit, 0, topK)
	for rows.Next() {
		var (
			chunkID    string
			documentID string
			kbID       string
			index      int
			content    string
			metadata   []byte
			score      float32
		)
		if err := rows.Scan(&chunkID, &documentID, &kbID, &index, &content, &metadata, &score); err != nil {
			return nil, fmt.Errorf("scan keyword search hit: %w", err)
		}
		result = append(result, corevector.SearchHit{
			ChunkID:         chunkID,
			DocumentID:      documentID,
			KnowledgeBaseID: kbID,
			Index:           index,
			Text:            content,
			Score:           score,
			Metadata:        unmarshalMetadata(metadata),
		})
	}
	return result, nil
}

func (s *VectorStore) SearchByMetadata(ctx context.Context, query string, knowledgeBaseIDs []string, topK int) ([]corevector.SearchHit, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres vector store db is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []corevector.SearchHit{}, nil
	}
	if topK <= 0 {
		topK = 5
	}

	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString(`
SELECT
    chunk_id,
    doc_id,
    kb_id,
    chunk_index,
    content,
    metadata,
    GREATEST(
        word_similarity(COALESCE(metadata->>'document_name', ''), ?),
        word_similarity(COALESCE(metadata->>'source_file_name', ''), ?),
        word_similarity(COALESCE(metadata->>'section', ''), ?)
    ) AS score
FROM t_knowledge_chunk_vector
WHERE GREATEST(
    word_similarity(COALESCE(metadata->>'document_name', ''), ?),
    word_similarity(COALESCE(metadata->>'source_file_name', ''), ?),
    word_similarity(COALESCE(metadata->>'section', ''), ?)
) > 0
`)
	args := []any{query, query, query, query, query, query}
	if len(knowledgeBaseIDs) > 0 {
		sqlBuilder.WriteString("AND kb_id IN ?\n")
		args = append(args, knowledgeBaseIDs)
	}
	sqlBuilder.WriteString("ORDER BY score DESC\nLIMIT ?")
	args = append(args, topK)

	queryResult := s.db.WithContext(ctx).Raw(sqlBuilder.String(), args...)

	rows, err := queryResult.Rows()
	if err != nil {
		return nil, fmt.Errorf("metadata search chunks: %w", err)
	}
	defer rows.Close()

	result := make([]corevector.SearchHit, 0, topK)
	for rows.Next() {
		var (
			chunkID    string
			documentID string
			kbID       string
			index      int
			content    string
			metadata   []byte
			score      float32
		)
		if err := rows.Scan(&chunkID, &documentID, &kbID, &index, &content, &metadata, &score); err != nil {
			return nil, fmt.Errorf("scan metadata search hit: %w", err)
		}
		result = append(result, corevector.SearchHit{
			ChunkID:         chunkID,
			DocumentID:      documentID,
			KnowledgeBaseID: kbID,
			Index:           index,
			Text:            content,
			Score:           score,
			Metadata:        unmarshalMetadata(metadata),
		})
	}
	return result, nil
}

func (s *VectorStore) UpdateChunk(ctx context.Context, chunk port.ChunkVector) error {
	return s.UpsertDocumentChunks(ctx, []port.ChunkVector{chunk})
}

func (s *VectorStore) Search(ctx context.Context, request corevector.SearchRequest) ([]corevector.SearchHit, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres vector store db is required")
	}
	if len(request.Vector) == 0 {
		return []corevector.SearchHit{}, nil
	}

	topK := request.TopK
	if topK <= 0 {
		topK = 5
	}

	vectorLiteral := formatVector(request.Vector)
	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString(`
SELECT chunk_id, doc_id, kb_id, chunk_index, content, metadata, 1 - (embedding <=> CAST(? AS vector)) AS score
FROM t_knowledge_chunk_vector
`)
	args := []any{vectorLiteral}
	if len(request.KnowledgeBaseIDs) > 0 {
		sqlBuilder.WriteString("WHERE kb_id IN ?\n")
		args = append(args, request.KnowledgeBaseIDs)
	}
	sqlBuilder.WriteString("ORDER BY embedding <=> CAST(? AS vector)\nLIMIT ?")
	args = append(args, vectorLiteral, topK)

	query := s.db.WithContext(ctx).Raw(sqlBuilder.String(), args...)

	rows, err := query.Rows()
	if err != nil {
		return nil, fmt.Errorf("search knowledge chunk vectors: %w", err)
	}
	defer rows.Close()

	result := make([]corevector.SearchHit, 0, topK)
	for rows.Next() {
		var (
			chunkID    string
			documentID string
			kbID       string
			index      int
			content    string
			metadata   []byte
			score      float32
		)
		if err := rows.Scan(&chunkID, &documentID, &kbID, &index, &content, &metadata, &score); err != nil {
			return nil, fmt.Errorf("scan vector search hit: %w", err)
		}
		if request.ScoreThreshold != nil && score < *request.ScoreThreshold {
			continue
		}

		result = append(result, corevector.SearchHit{
			ChunkID:         chunkID,
			DocumentID:      documentID,
			KnowledgeBaseID: kbID,
			Index:           index,
			Text:            content,
			Score:           score,
			Metadata:        unmarshalMetadata(metadata),
		})
	}
	return result, nil
}

func formatVector(values []float32) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func normalizeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func unmarshalMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return map[string]any{}
	}
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}
