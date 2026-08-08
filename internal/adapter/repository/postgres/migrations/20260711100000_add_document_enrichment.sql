-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_chunk ADD COLUMN IF NOT EXISTS record_type VARCHAR(16) NOT NULL DEFAULT 'child';
ALTER TABLE t_knowledge_chunk ADD COLUMN IF NOT EXISTS parent_chunk_id VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunk_parent_chunk_id ON t_knowledge_chunk(parent_chunk_id);

ALTER TABLE t_knowledge_document ADD COLUMN IF NOT EXISTS summary TEXT;
ALTER TABLE t_knowledge_document ADD COLUMN IF NOT EXISTS summary_status VARCHAR(16) NOT NULL DEFAULT 'none';
ALTER TABLE t_knowledge_document ADD COLUMN IF NOT EXISTS summary_error_message VARCHAR(512);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_knowledge_chunk_parent_chunk_id;
ALTER TABLE t_knowledge_chunk DROP COLUMN IF EXISTS parent_chunk_id;
ALTER TABLE t_knowledge_chunk DROP COLUMN IF EXISTS record_type;
ALTER TABLE t_knowledge_document DROP COLUMN IF EXISTS summary_error_message;
ALTER TABLE t_knowledge_document DROP COLUMN IF EXISTS summary_status;
ALTER TABLE t_knowledge_document DROP COLUMN IF EXISTS summary;
-- +goose StatementEnd
