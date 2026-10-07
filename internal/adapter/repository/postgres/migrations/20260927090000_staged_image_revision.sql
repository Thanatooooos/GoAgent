-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_document_revision
    ADD COLUMN IF NOT EXISTS base_revision_id VARCHAR(36);

ALTER TABLE t_knowledge_image_task
    DROP CONSTRAINT IF EXISTS t_knowledge_image_task_operation_check;
ALTER TABLE t_knowledge_image_task
    ADD CONSTRAINT t_knowledge_image_task_operation_check
    CHECK (operation IN ('ocr', 'caption', 'inventory', 'cleanup', 'index'));

CREATE UNIQUE INDEX IF NOT EXISTS uk_knowledge_image_active_index_task
    ON t_knowledge_image_task (revision_id, occurrence_id, operation)
    WHERE operation = 'index' AND status IN ('pending', 'running');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Historical revisions and index tasks must survive a rollback.
SELECT 1;
-- +goose StatementEnd
