CREATE TABLE IF NOT EXISTS t_runtime_episode (
    id                          VARCHAR(64) PRIMARY KEY,
    runtime_session_id          VARCHAR(64) NOT NULL REFERENCES t_runtime_session(id),
    conversation_id             VARCHAR(64) NOT NULL,
    user_id                     VARCHAR(64) NOT NULL,
    source_user_message_id      VARCHAR(64) NOT NULL,
    source_assistant_message_id VARCHAR(64) NOT NULL DEFAULT '',
    summary                     TEXT NOT NULL,
    topics_json                 TEXT NOT NULL DEFAULT '[]',
    importance                  VARCHAR(16) NOT NULL,
    mentioned_start             TIMESTAMP NULL,
    mentioned_end               TIMESTAMP NULL,
    status                      VARCHAR(16) NOT NULL,
    create_time                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_runtime_episode_user_status_time ON t_runtime_episode (user_id, status, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_episode_session ON t_runtime_episode (runtime_session_id, status);

CREATE TABLE IF NOT EXISTS t_runtime_episode_embedding (
    episode_id  VARCHAR(64) PRIMARY KEY REFERENCES t_runtime_episode(id),
    embedding   vector NOT NULL,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
