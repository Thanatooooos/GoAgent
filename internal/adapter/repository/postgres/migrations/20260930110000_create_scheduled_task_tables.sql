CREATE TABLE IF NOT EXISTS t_scheduled_task (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL,
    current_version INTEGER NOT NULL,
    conversation_id VARCHAR(64),
    confirmed_at TIMESTAMPTZ NOT NULL,
    next_due_at TIMESTAMPTZ,
    last_reported_at TIMESTAMPTZ,
    last_attempted_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_scheduled_task_user_status ON t_scheduled_task (user_id, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_scheduled_task_due ON t_scheduled_task (next_due_at) WHERE status = 'active' AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uk_scheduled_task_conversation ON t_scheduled_task (conversation_id) WHERE conversation_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS t_scheduled_task_version (
    task_id VARCHAR(64) NOT NULL REFERENCES t_scheduled_task(id),
    version INTEGER NOT NULL,
    prompt TEXT NOT NULL,
    schedule_json JSONB NOT NULL,
    report_mode VARCHAR(24) NOT NULL,
    condition_kind VARCHAR(16) NOT NULL,
    knowledge_base_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_web_domains_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_tool_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    confirmed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (task_id, version)
);

CREATE TABLE IF NOT EXISTS t_scheduled_task_draft (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    originating_conversation_id VARCHAR(64),
    task_id VARCHAR(64),
    base_version INTEGER,
    proposed_config_json JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_scheduled_task_draft_user ON t_scheduled_task_draft (user_id, expires_at);

CREATE TABLE IF NOT EXISTS t_scheduled_task_occurrence (
    id VARCHAR(64) PRIMARY KEY,
    task_id VARCHAR(64) NOT NULL REFERENCES t_scheduled_task(id),
    version INTEGER NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    deadline_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL,
    result_signal VARCHAR(16),
    result_json JSONB,
    published_message_id VARCHAR(64),
    lease_owner VARCHAR(64),
    lease_until TIMESTAMPTZ,
    create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_scheduled_task_occurrence UNIQUE (task_id, version, scheduled_at),
    FOREIGN KEY (task_id, version) REFERENCES t_scheduled_task_version(task_id, version)
);
CREATE INDEX IF NOT EXISTS idx_scheduled_occurrence_retry ON t_scheduled_task_occurrence (status, lease_until, deadline_at);

CREATE TABLE IF NOT EXISTS t_scheduled_task_attempt (
    id VARCHAR(64) PRIMARY KEY,
    occurrence_id VARCHAR(64) NOT NULL REFERENCES t_scheduled_task_occurrence(id),
    runtime_session_id VARCHAR(64),
    status VARCHAR(20) NOT NULL,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_scheduled_attempt_occurrence ON t_scheduled_task_attempt (occurrence_id, started_at);

CREATE TABLE IF NOT EXISTS t_scheduled_task_notification (
    id VARCHAR(64) PRIMARY KEY,
    task_id VARCHAR(64) NOT NULL REFERENCES t_scheduled_task(id),
    occurrence_id VARCHAR(64),
    event_key VARCHAR(128) NOT NULL,
    conversation_id VARCHAR(64) NOT NULL,
    message_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_scheduled_task_notification_event UNIQUE (task_id, event_key)
);

CREATE TABLE IF NOT EXISTS t_conversation_unread (
    user_id VARCHAR(64) NOT NULL,
    conversation_id VARCHAR(64) NOT NULL,
    unread_count INTEGER NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, conversation_id)
);
