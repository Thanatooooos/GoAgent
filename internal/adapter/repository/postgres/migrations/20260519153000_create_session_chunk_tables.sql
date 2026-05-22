CREATE TABLE IF NOT EXISTS t_session_chunk (
    id              VARCHAR(20) NOT NULL PRIMARY KEY,
    conversation_id VARCHAR(20) NOT NULL,
    message_id      VARCHAR(20) NOT NULL,
    user_id         VARCHAR(20) NOT NULL,
    chunk_index     INTEGER NOT NULL,
    content         TEXT NOT NULL,
    content_summary TEXT,
    token_estimate  INTEGER NOT NULL DEFAULT 0,
    create_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT DEFAULT 0,
    CONSTRAINT uk_session_chunk_message_index UNIQUE (message_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_session_chunk_conversation_time ON t_session_chunk (conversation_id, user_id, create_time);
CREATE INDEX IF NOT EXISTS idx_session_chunk_message ON t_session_chunk (message_id);

CREATE TABLE IF NOT EXISTS t_session_chunk_embedding (
    chunk_id     VARCHAR(20) NOT NULL PRIMARY KEY,
    embedding    VECTOR NOT NULL,
    create_time  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
