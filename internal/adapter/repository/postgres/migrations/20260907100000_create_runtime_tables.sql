CREATE TABLE IF NOT EXISTS t_runtime_session (
    id              VARCHAR(64) PRIMARY KEY,
    conversation_id VARCHAR(64) NOT NULL,
    user_message_id VARCHAR(64) NOT NULL,
    user_id         VARCHAR(64) NOT NULL,
    trace_id        VARCHAR(64) NOT NULL,
    status          VARCHAR(32) NOT NULL,
    next_sequence   BIGINT NOT NULL DEFAULT 1,
    create_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_runtime_session_message UNIQUE (conversation_id, user_message_id)
);
CREATE INDEX IF NOT EXISTS idx_runtime_session_user ON t_runtime_session (user_id);
CREATE INDEX IF NOT EXISTS idx_runtime_session_trace ON t_runtime_session (trace_id);

CREATE TABLE IF NOT EXISTS t_runtime_journal (
    id                 VARCHAR(64) PRIMARY KEY,
    runtime_session_id VARCHAR(64) NOT NULL REFERENCES t_runtime_session(id),
    sequence           BIGINT NOT NULL,
    conversation_id    VARCHAR(64) NOT NULL,
    user_message_id    VARCHAR(64) NOT NULL,
    trace_id           VARCHAR(64) NOT NULL,
    event_type         VARCHAR(32) NOT NULL,
    tool_call_id       VARCHAR(64) NOT NULL DEFAULT '',
    tool_name          VARCHAR(128) NOT NULL DEFAULT '',
    tool_state         VARCHAR(16) NOT NULL DEFAULT '',
    evidence_json      TEXT NOT NULL DEFAULT '[]',
    detail             TEXT NOT NULL DEFAULT '',
    create_time        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_runtime_journal_sequence UNIQUE (runtime_session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_runtime_journal_session ON t_runtime_journal (runtime_session_id, sequence);
CREATE INDEX IF NOT EXISTS idx_runtime_journal_tool ON t_runtime_journal (runtime_session_id, tool_call_id, sequence);
CREATE INDEX IF NOT EXISTS idx_runtime_journal_conversation ON t_runtime_journal (conversation_id, user_message_id, sequence);
CREATE INDEX IF NOT EXISTS idx_runtime_journal_trace ON t_runtime_journal (trace_id);
