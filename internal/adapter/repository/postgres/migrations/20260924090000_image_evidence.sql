-- +goose Up
-- +goose StatementBegin
ALTER TABLE t_knowledge_document
    ADD COLUMN IF NOT EXISTS active_revision_id VARCHAR(36),
    ADD COLUMN IF NOT EXISTS image_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS image_completed_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS image_failed_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS t_knowledge_document_revision (
    id VARCHAR(36) PRIMARY KEY,
    doc_id VARCHAR(20) NOT NULL,
    source_hash VARCHAR(64) NOT NULL,
    parser_type VARCHAR(32) NOT NULL,
    image_inventory_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(16) NOT NULL DEFAULT 'building',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    UNIQUE (doc_id, source_hash, id)
);
CREATE INDEX IF NOT EXISTS idx_knowledge_revision_doc ON t_knowledge_document_revision (doc_id, created_at DESC);

CREATE TABLE IF NOT EXISTS t_knowledge_image_occurrence (
    id VARCHAR(36) PRIMARY KEY,
    doc_id VARCHAR(20) NOT NULL,
    revision_id VARCHAR(36) NOT NULL REFERENCES t_knowledge_document_revision(id),
    ordinal INTEGER NOT NULL,
    original_ref TEXT,
    page_number INTEGER,
    original_object_key TEXT,
    original_mime_type VARCHAR(64),
    original_sha256 VARCHAR(64),
    processing_object_key TEXT,
    processing_mime_type VARCHAR(64),
    source_error VARCHAR(64),
    active_evidence_id VARCHAR(36),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (revision_id, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_knowledge_image_doc ON t_knowledge_image_occurrence (doc_id, revision_id);

CREATE TABLE IF NOT EXISTS t_knowledge_image_evidence (
    id VARCHAR(36) PRIMARY KEY,
    occurrence_id VARCHAR(36) NOT NULL REFERENCES t_knowledge_image_occurrence(id),
    revision_id VARCHAR(36) NOT NULL REFERENCES t_knowledge_document_revision(id),
    ocr_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    ocr_text TEXT,
    ocr_error VARCHAR(64),
    caption_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    caption_text TEXT,
    caption_error VARCHAR(64),
    caption_model VARCHAR(64),
    prompt_version VARCHAR(64),
    adjacent_text TEXT,
    searchable_text TEXT,
    vector_published BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_knowledge_image_evidence_occurrence ON t_knowledge_image_evidence (occurrence_id, created_at DESC);

CREATE TABLE IF NOT EXISTS t_knowledge_image_task (
    id VARCHAR(36) PRIMARY KEY,
    doc_id VARCHAR(20) NOT NULL,
    revision_id VARCHAR(36) NOT NULL REFERENCES t_knowledge_document_revision(id),
    occurrence_id VARCHAR(36) REFERENCES t_knowledge_image_occurrence(id),
    operation VARCHAR(16) NOT NULL CHECK (operation IN ('ocr', 'caption', 'inventory', 'cleanup')),
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner VARCHAR(128),
    lease_until TIMESTAMPTZ,
    error_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_knowledge_image_active_task
    ON t_knowledge_image_task (revision_id, occurrence_id, operation)
    WHERE operation IN ('ocr', 'caption') AND status IN ('pending', 'running');
CREATE INDEX IF NOT EXISTS idx_knowledge_image_task_due
    ON t_knowledge_image_task (next_run_at, lease_until)
    WHERE status IN ('pending', 'running');

CREATE TABLE IF NOT EXISTS t_knowledge_image_object_cleanup (
    object_key TEXT PRIMARY KEY,
    doc_id VARCHAR(20) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Image evidence can be referenced by historic chats. Rollback disables the
-- new reader and worker but deliberately preserves evidence and object keys.
SELECT 1;
-- +goose StatementEnd
