-- +goose Up
-- +goose StatementBegin
-- GIN trigram indexes for word_similarity keyword and metadata search.
-- Without these, word_similarity scans the entire t_knowledge_chunk_vector table
-- and computes trigram similarity per row, which degrades to ~400ms per query
-- even with only 5000 rows.

-- Keyword search on chunk content
CREATE INDEX IF NOT EXISTS idx_chunk_vector_content_trgm
    ON t_knowledge_chunk_vector USING GIN (content gin_trgm_ops);

-- Metadata search: word_similarity on document_name, source_file_name, section
CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_docname_trgm
    ON t_knowledge_chunk_vector USING GIN ((metadata->>'document_name') gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_filename_trgm
    ON t_knowledge_chunk_vector USING GIN ((metadata->>'source_file_name') gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_section_trgm
    ON t_knowledge_chunk_vector USING GIN ((metadata->>'section') gin_trgm_ops);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_chunk_vector_content_trgm;
DROP INDEX IF EXISTS idx_chunk_vector_meta_docname_trgm;
DROP INDEX IF EXISTS idx_chunk_vector_meta_filename_trgm;
DROP INDEX IF EXISTS idx_chunk_vector_meta_section_trgm;
-- +goose StatementEnd
