-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_document_revision
    ADD COLUMN IF NOT EXISTS ocr_summary_hash VARCHAR(32);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
