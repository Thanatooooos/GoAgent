-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_chunk_vector
    ADD COLUMN IF NOT EXISTS content_lexemes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS metadata_document_name_lexemes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS metadata_source_file_name_lexemes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS metadata_section_lexemes TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chunk_vector_content_lexemes_tsv
    ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', content_lexemes));

CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_docname_lexemes_tsv
    ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', metadata_document_name_lexemes));

CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_filename_lexemes_tsv
    ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', metadata_source_file_name_lexemes));

CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_section_lexemes_tsv
    ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', metadata_section_lexemes));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_chunk_vector_content_lexemes_tsv;
DROP INDEX IF EXISTS idx_chunk_vector_meta_docname_lexemes_tsv;
DROP INDEX IF EXISTS idx_chunk_vector_meta_filename_lexemes_tsv;
DROP INDEX IF EXISTS idx_chunk_vector_meta_section_lexemes_tsv;

ALTER TABLE t_knowledge_chunk_vector
    DROP COLUMN IF EXISTS content_lexemes,
    DROP COLUMN IF EXISTS metadata_document_name_lexemes,
    DROP COLUMN IF EXISTS metadata_source_file_name_lexemes,
    DROP COLUMN IF EXISTS metadata_section_lexemes;
-- +goose StatementEnd
