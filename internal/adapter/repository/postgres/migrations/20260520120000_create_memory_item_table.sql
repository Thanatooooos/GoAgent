CREATE TABLE IF NOT EXISTS t_memory_item (
    id VARCHAR(20) PRIMARY KEY,
    user_id VARCHAR(20) NOT NULL,
    scope_type VARCHAR(16) NOT NULL,
    scope_id VARCHAR(20),
    memory_type VARCHAR(16) NOT NULL,
    source_message_id VARCHAR(20),
    content TEXT NOT NULL,
    summary TEXT,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1,
    status VARCHAR(16) NOT NULL,
    last_confirmed_at TIMESTAMP,
    expires_at TIMESTAMP,
    created_by VARCHAR(20) NOT NULL,
    updated_by VARCHAR(20) NOT NULL,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted SMALLINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_memory_item_user_scope_status_time
    ON t_memory_item (user_id, scope_type, status, update_time DESC);

CREATE INDEX IF NOT EXISTS idx_memory_item_user_scope_id_status
    ON t_memory_item (user_id, scope_type, scope_id, status);

CREATE INDEX IF NOT EXISTS idx_memory_item_source_message
    ON t_memory_item (source_message_id);
