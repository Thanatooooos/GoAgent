-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_image_evidence
    ADD COLUMN IF NOT EXISTS caption_input_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS caption_output_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS caption_latency_ms INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Preserve published evidence and its usage diagnostics during rollback.
SELECT 1;
-- +goose StatementEnd
