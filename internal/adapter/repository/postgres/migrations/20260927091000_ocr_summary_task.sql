-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_image_task
    DROP CONSTRAINT IF EXISTS t_knowledge_image_task_operation_check;
ALTER TABLE t_knowledge_image_task
    ADD CONSTRAINT t_knowledge_image_task_operation_check
    CHECK (operation IN ('ocr', 'caption', 'inventory', 'cleanup', 'index', 'summary'));
CREATE UNIQUE INDEX IF NOT EXISTS uk_knowledge_image_active_summary_task
    ON t_knowledge_image_task (revision_id, operation)
    WHERE operation = 'summary' AND status IN ('pending', 'running');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
