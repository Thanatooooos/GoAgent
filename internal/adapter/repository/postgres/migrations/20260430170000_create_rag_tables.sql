CREATE TABLE IF NOT EXISTS t_conversation (
    id              VARCHAR(20) NOT NULL PRIMARY KEY,
    conversation_id VARCHAR(20) NOT NULL,
    user_id         VARCHAR(20) NOT NULL,
    title           VARCHAR(128) NOT NULL,
    last_time       TIMESTAMP,
    create_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT DEFAULT 0,
    CONSTRAINT uk_conversation_user UNIQUE (conversation_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_user_time ON t_conversation (user_id, last_time);

CREATE TABLE IF NOT EXISTS t_conversation_summary (
    id              VARCHAR(20) NOT NULL PRIMARY KEY,
    conversation_id VARCHAR(20) NOT NULL,
    user_id         VARCHAR(20) NOT NULL,
    last_message_id VARCHAR(20) NOT NULL,
    content         TEXT NOT NULL,
    create_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_conv_user ON t_conversation_summary (conversation_id, user_id);

CREATE TABLE IF NOT EXISTS t_message (
    id                VARCHAR(20) NOT NULL PRIMARY KEY,
    conversation_id   VARCHAR(20) NOT NULL,
    user_id           VARCHAR(20) NOT NULL,
    role              VARCHAR(16) NOT NULL,
    content           TEXT NOT NULL,
    thinking_content  TEXT,
    thinking_duration INTEGER,
    create_time       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted           SMALLINT DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_conversation_user_time ON t_message (conversation_id, user_id, create_time);
CREATE INDEX IF NOT EXISTS idx_conversation_summary ON t_message (conversation_id, user_id, create_time);

CREATE TABLE IF NOT EXISTS t_message_feedback (
    id              VARCHAR(20) NOT NULL PRIMARY KEY,
    message_id      VARCHAR(20) NOT NULL,
    conversation_id VARCHAR(20) NOT NULL,
    user_id         VARCHAR(20) NOT NULL,
    vote            SMALLINT NOT NULL,
    reason          VARCHAR(255),
    comment         VARCHAR(1024),
    create_time     TIMESTAMP NOT NULL,
    update_time     TIMESTAMP NOT NULL,
    deleted         SMALLINT NOT NULL DEFAULT 0,
    CONSTRAINT uk_msg_user UNIQUE (message_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_conversation_id ON t_message_feedback (conversation_id);
CREATE INDEX IF NOT EXISTS idx_user_id ON t_message_feedback (user_id);

CREATE TABLE IF NOT EXISTS t_rag_trace_run (
    id              VARCHAR(20) NOT NULL PRIMARY KEY,
    trace_id        VARCHAR(64) NOT NULL,
    trace_name      VARCHAR(128),
    entry_method    VARCHAR(256),
    conversation_id VARCHAR(20),
    task_id         VARCHAR(20),
    user_id         VARCHAR(20),
    status          VARCHAR(16) NOT NULL DEFAULT 'RUNNING',
    error_message   VARCHAR(1000),
    start_time      TIMESTAMP(3),
    end_time        TIMESTAMP(3),
    duration_ms     BIGINT,
    extra_data      TEXT,
    create_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT DEFAULT 0,
    CONSTRAINT uk_run_id UNIQUE (trace_id)
);
CREATE INDEX IF NOT EXISTS idx_task_id ON t_rag_trace_run (task_id);
CREATE INDEX IF NOT EXISTS idx_user_id_trace ON t_rag_trace_run (user_id);

CREATE TABLE IF NOT EXISTS t_rag_trace_node (
    id             VARCHAR(20) NOT NULL PRIMARY KEY,
    trace_id       VARCHAR(20) NOT NULL,
    node_id        VARCHAR(20) NOT NULL,
    parent_node_id VARCHAR(20),
    depth          INTEGER DEFAULT 0,
    node_type      VARCHAR(16),
    node_name      VARCHAR(128),
    class_name     VARCHAR(256),
    method_name    VARCHAR(128),
    status         VARCHAR(16) NOT NULL DEFAULT 'RUNNING',
    error_message  VARCHAR(1000),
    start_time     TIMESTAMP(3),
    end_time       TIMESTAMP(3),
    duration_ms    BIGINT,
    extra_data     TEXT,
    create_time    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted        SMALLINT DEFAULT 0,
    CONSTRAINT uk_run_node UNIQUE (trace_id, node_id)
);
