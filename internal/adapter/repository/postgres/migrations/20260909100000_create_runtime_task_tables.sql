CREATE TABLE IF NOT EXISTS t_runtime_task_session (
    id VARCHAR(64) PRIMARY KEY,
    task_type VARCHAR(64) NOT NULL,
    task_id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    next_sequence BIGINT NOT NULL DEFAULT 1,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_runtime_task_session UNIQUE (task_type, task_id)
);
CREATE TABLE IF NOT EXISTS t_runtime_task_journal (
    id VARCHAR(64) PRIMARY KEY,
    runtime_task_session_id VARCHAR(64) NOT NULL REFERENCES t_runtime_task_session(id),
    sequence BIGINT NOT NULL,
    event_type VARCHAR(32) NOT NULL,
    tool_call_id VARCHAR(64) NOT NULL DEFAULT '',
    tool_name VARCHAR(128) NOT NULL DEFAULT '',
    tool_state VARCHAR(16) NOT NULL DEFAULT '',
    evidence_json TEXT NOT NULL DEFAULT '[]',
    detail TEXT NOT NULL DEFAULT '',
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_runtime_task_journal_sequence UNIQUE (runtime_task_session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_runtime_task_journal_session ON t_runtime_task_journal (runtime_task_session_id, sequence);
