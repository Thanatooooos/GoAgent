-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_message
    ADD COLUMN IF NOT EXISTS sources JSONB NOT NULL DEFAULT '[]'::jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE t_message DROP COLUMN IF EXISTS sources;
-- +goose StatementEnd
