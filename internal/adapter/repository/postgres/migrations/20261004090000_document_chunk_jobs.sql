ALTER TABLE t_knowledge_document ADD COLUMN chunk_epoch BIGINT NOT NULL DEFAULT 0;
ALTER TABLE t_knowledge_document ADD COLUMN current_chunk_job_id VARCHAR(20);
CREATE TABLE t_document_chunk_job (
    id VARCHAR(20) PRIMARY KEY,
    document_id VARCHAR(20) NOT NULL REFERENCES t_knowledge_document(id),
    epoch BIGINT NOT NULL,
    triggered_by VARCHAR(20) NOT NULL,
    document_snapshot JSONB NOT NULL,
    source_file_url VARCHAR(1024) NOT NULL,
    refreshed BOOLEAN NOT NULL DEFAULT FALSE,
    state VARCHAR(16) NOT NULL CHECK(state IN ('pending','running','completed','failed','interrupted')),
    owner VARCHAR(64),
    lease_until TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(document_id,epoch)
);
CREATE UNIQUE INDEX uk_document_chunk_job_active ON t_document_chunk_job(document_id) WHERE state IN ('pending','running');
CREATE INDEX idx_document_chunk_job_pending ON t_document_chunk_job(created_at) WHERE state='pending';
CREATE INDEX idx_document_chunk_job_lease ON t_document_chunk_job(lease_until) WHERE state='running';
