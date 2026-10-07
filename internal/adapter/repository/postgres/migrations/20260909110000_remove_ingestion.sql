-- +goose Up
-- +goose StatementBegin
UPDATE t_knowledge_document SET process_mode = 'chunk' WHERE process_mode = 'pipeline';
UPDATE t_knowledge_document_chunk_log SET process_mode = 'chunk' WHERE process_mode = 'pipeline';
ALTER TABLE t_knowledge_document DROP COLUMN IF EXISTS pipeline_id;
ALTER TABLE t_knowledge_document_chunk_log DROP COLUMN IF EXISTS pipeline_id;
DROP TABLE IF EXISTS t_ingestion_task_node;
DROP TABLE IF EXISTS t_ingestion_task;
DROP TABLE IF EXISTS t_ingestion_pipeline;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Historical ingestion data is intentionally not restored.
ALTER TABLE t_knowledge_document ADD COLUMN IF NOT EXISTS pipeline_id VARCHAR(20);
ALTER TABLE t_knowledge_document_chunk_log ADD COLUMN IF NOT EXISTS pipeline_id VARCHAR(20);
-- +goose StatementEnd
