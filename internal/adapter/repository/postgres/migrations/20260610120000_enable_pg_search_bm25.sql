-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pg_search;

CREATE INDEX IF NOT EXISTS idx_chunk_vector_bm25
ON t_knowledge_chunk_vector
USING bm25 (
    chunk_id,
    (content::pdb.chinese_compatible)
)
WITH (key_field='chunk_id');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_chunk_vector_bm25;
-- +goose StatementEnd
