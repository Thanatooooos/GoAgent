-- +goose Up
-- +goose StatementBegin
-- Drop keyword-specific tsvector and trigram indexes no longer needed by BM25.
-- Metadata lexeme/trigram indexes are preserved for SearchByMetadata.
DROP INDEX IF EXISTS idx_chunk_vector_content_lexemes_tsv;
DROP INDEX IF EXISTS idx_chunk_vector_content_trgm;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Recreate keyword indexes if rollback is needed.
CREATE INDEX IF NOT EXISTS idx_chunk_vector_content_lexemes_tsv
ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', content_lexemes));
CREATE INDEX IF NOT EXISTS idx_chunk_vector_content_trgm
ON t_knowledge_chunk_vector USING GIN (content gin_trgm_ops);
-- +goose StatementEnd
