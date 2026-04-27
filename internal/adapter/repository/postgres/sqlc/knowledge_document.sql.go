package sqlc

import "context"

const countChunkedDocumentsByKnowledgeBaseID = `-- name: CountChunkedDocumentsByKnowledgeBaseID :one
SELECT COUNT(DISTINCT kd.id)
FROM t_knowledge_document kd
WHERE kd.kb_id = $1
  AND kd.deleted = 0
  AND EXISTS (
    SELECT 1
    FROM t_knowledge_chunk kc
    WHERE kc.doc_id = kd.id
      AND kc.deleted = 0
  )
`

func (q *Queries) CountChunkedDocumentsByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int64, error) {
	row := q.db.QueryRow(ctx, countChunkedDocumentsByKnowledgeBaseID, knowledgeBaseID)
	var count int64
	err := row.Scan(&count)
	return count, err
}
