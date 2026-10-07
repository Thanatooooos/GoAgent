CREATE TABLE t_runtime_chat_execution (
    task_id VARCHAR(64) PRIMARY KEY,
    conversation_id VARCHAR(64) NOT NULL,
    user_message_id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','running','completed','failed','cancelled','interrupted')),
    owner VARCHAR(64),
    epoch BIGINT NOT NULL DEFAULT 0,
    lease_until TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(conversation_id,user_message_id)
);
CREATE INDEX idx_runtime_chat_execution_expired ON t_runtime_chat_execution(lease_until) WHERE state IN ('pending','running');
