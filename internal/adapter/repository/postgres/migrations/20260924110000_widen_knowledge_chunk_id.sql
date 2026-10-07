-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_chunk ALTER COLUMN id TYPE VARCHAR(64);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Historical text citations may contain versioned IDs over 20 characters.
SELECT 1;
-- +goose StatementEnd
