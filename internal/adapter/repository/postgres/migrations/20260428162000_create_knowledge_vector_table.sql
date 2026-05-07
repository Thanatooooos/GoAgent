-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS t_knowledge_chunk_vector (
    chunk_id VARCHAR(20) NOT NULL PRIMARY KEY,
    doc_id   VARCHAR(20) NOT NULL,
    kb_id    VARCHAR(20) NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding VECTOR,
    metadata JSONB,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chunk_vector_doc_id ON t_knowledge_chunk_vector (doc_id);
CREATE INDEX IF NOT EXISTS idx_chunk_vector_kb_id ON t_knowledge_chunk_vector (kb_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- DROP TABLE IF EXISTS t_knowledge_chunk_vector;
-- +goose StatementEnd
