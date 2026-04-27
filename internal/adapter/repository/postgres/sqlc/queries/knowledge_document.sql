-- name: CountChunkedDocumentsByKnowledgeBaseID :one
SELECT COUNT(DISTINCT kd.id)
FROM t_knowledge_document kd
WHERE kd.kb_id = $1
  AND kd.deleted = 0
  AND EXISTS (
    SELECT 1
    FROM t_knowledge_chunk kc
    WHERE kc.doc_id = kd.id
      AND kc.deleted = 0
  );
