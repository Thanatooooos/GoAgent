-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_attribute
        WHERE attrelid = to_regclass('t_knowledge_chunk_vector')
          AND attname = 'embedding'
          AND atttypid = 'vector'::regtype
          AND atttypmod > 0
          AND NOT attisdropped
    ) THEN
        EXECUTE '
            CREATE INDEX IF NOT EXISTS idx_chunk_vector_embedding_hnsw
            ON t_knowledge_chunk_vector
            USING hnsw (embedding vector_cosine_ops)
        ';
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_chunk_vector_embedding_hnsw;
-- +goose StatementEnd
