CREATE TABLE IF NOT EXISTS t_user_memory_profile (
    user_id VARCHAR(64) PRIMARY KEY,
    content_markdown TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    last_observed_at TIMESTAMP NULL,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS t_conversation_profile_state (
    conversation_id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    last_observed_message_id VARCHAR(64) NOT NULL DEFAULT '',
    processing_to_message_id VARCHAR(64) NOT NULL DEFAULT '',
    last_observed_at TIMESTAMP NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    next_run_at TIMESTAMP NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_conversation_profile_state_due
    ON t_conversation_profile_state (status, next_run_at);
