CREATE TABLE t_runtime_chat_publication (
    runtime_session_id VARCHAR(64) PRIMARY KEY REFERENCES t_runtime_session(id),
    state VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','published','abandoned')),
    assistant_message_id VARCHAR(20) UNIQUE REFERENCES t_message(id),
    finish_event_id VARCHAR(64) UNIQUE REFERENCES t_runtime_journal(id),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((state='published') = (assistant_message_id IS NOT NULL AND finish_event_id IS NOT NULL))
);
CREATE INDEX idx_runtime_chat_publication_due ON t_runtime_chat_publication(next_attempt_at) WHERE state='pending';
