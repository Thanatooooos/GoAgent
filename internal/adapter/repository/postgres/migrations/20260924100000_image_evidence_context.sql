-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_image_occurrence
    ADD COLUMN IF NOT EXISTS adjacent_text TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Keep historical citation context after application rollback.
SELECT 1;
-- +goose StatementEnd
