CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS t_knowledge_base (
    id              VARCHAR(20)  NOT NULL PRIMARY KEY,
    name            VARCHAR(128) NOT NULL,
    embedding_model VARCHAR(64)  NOT NULL,
    collection_name VARCHAR(64)  NOT NULL,
    created_by      VARCHAR(20)  NOT NULL,
    updated_by      VARCHAR(20),
    create_time     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT     NOT NULL DEFAULT 0,
    CONSTRAINT uk_collection_name UNIQUE (collection_name)
);
CREATE INDEX IF NOT EXISTS idx_kb_name ON t_knowledge_base (name);

CREATE TABLE IF NOT EXISTS t_knowledge_document (
    id               VARCHAR(20)   NOT NULL PRIMARY KEY,
    kb_id            VARCHAR(20)   NOT NULL,
    doc_name         VARCHAR(256)  NOT NULL,
    enabled          SMALLINT      NOT NULL DEFAULT 1,
    chunk_count      INTEGER       DEFAULT 0,
    file_url         VARCHAR(1024) NOT NULL,
    file_type        VARCHAR(16)   NOT NULL,
    file_size        BIGINT,
    process_mode     VARCHAR(16)   DEFAULT 'chunk',
    status           VARCHAR(16)   NOT NULL DEFAULT 'pending',
    source_type      VARCHAR(16),
    source_location  VARCHAR(1024),
    schedule_enabled SMALLINT,
    schedule_cron    VARCHAR(64),
    chunk_strategy   VARCHAR(32),
    chunk_config     JSONB,
    pipeline_id      VARCHAR(20),
    created_by       VARCHAR(20)   NOT NULL,
    updated_by       VARCHAR(20),
    create_time      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted          SMALLINT      NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_kb_id ON t_knowledge_document (kb_id);

CREATE TABLE IF NOT EXISTS t_knowledge_chunk (
    id           VARCHAR(20)  NOT NULL PRIMARY KEY,
    kb_id        VARCHAR(20)  NOT NULL,
    doc_id       VARCHAR(20)  NOT NULL,
    chunk_index  INTEGER      NOT NULL,
    content      TEXT         NOT NULL,
    content_hash VARCHAR(64),
    char_count   INTEGER,
    token_count  INTEGER,
    enabled      SMALLINT     NOT NULL DEFAULT 1,
    created_by   VARCHAR(20)  NOT NULL,
    updated_by   VARCHAR(20),
    create_time  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted      SMALLINT     NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_doc_id ON t_knowledge_chunk (doc_id);

CREATE TABLE IF NOT EXISTS t_knowledge_document_chunk_log (
    id               VARCHAR(20) NOT NULL PRIMARY KEY,
    doc_id           VARCHAR(20) NOT NULL,
    status           VARCHAR(16) NOT NULL,
    process_mode     VARCHAR(16),
    chunk_strategy   VARCHAR(16),
    pipeline_id      VARCHAR(20),
    extract_duration BIGINT,
    chunk_duration   BIGINT,
    embed_duration   BIGINT,
    persist_duration BIGINT,
    total_duration   BIGINT,
    chunk_count      INTEGER,
    error_message    TEXT,
    start_time       TIMESTAMP,
    end_time         TIMESTAMP,
    create_time      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_doc_id_log ON t_knowledge_document_chunk_log (doc_id);

CREATE TABLE IF NOT EXISTS t_knowledge_document_schedule (
    id                VARCHAR(20) NOT NULL PRIMARY KEY,
    doc_id            VARCHAR(20) NOT NULL,
    kb_id             VARCHAR(20) NOT NULL,
    cron_expr         VARCHAR(64) NOT NULL,
    enabled           SMALLINT    NOT NULL DEFAULT 1,
    next_run_time     TIMESTAMP,
    last_run_time     TIMESTAMP,
    last_success_time TIMESTAMP,
    last_status       VARCHAR(16),
    last_error        TEXT,
    last_etag         VARCHAR(256),
    last_modified     VARCHAR(128),
    last_content_hash VARCHAR(64),
    lock_owner        VARCHAR(128),
    lock_until        TIMESTAMP,
    create_time       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time       TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_schedule_doc_id ON t_knowledge_document_schedule (doc_id);
CREATE INDEX IF NOT EXISTS idx_next_run ON t_knowledge_document_schedule (next_run_time);
CREATE INDEX IF NOT EXISTS idx_lock_until ON t_knowledge_document_schedule (lock_until);

CREATE TABLE IF NOT EXISTS t_knowledge_document_schedule_exec (
    id            VARCHAR(20) NOT NULL PRIMARY KEY,
    schedule_id   VARCHAR(20) NOT NULL,
    doc_id        VARCHAR(20) NOT NULL,
    kb_id         VARCHAR(20) NOT NULL,
    status        VARCHAR(16) NOT NULL,
    message       VARCHAR(512),
    start_time    TIMESTAMP,
    end_time      TIMESTAMP,
    file_name     VARCHAR(256),
    file_size     BIGINT,
    content_hash  VARCHAR(64),
    etag          VARCHAR(256),
    last_modified VARCHAR(128),
    create_time   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_schedule_time ON t_knowledge_document_schedule_exec (schedule_id, start_time);
CREATE INDEX IF NOT EXISTS idx_doc_id_exec ON t_knowledge_document_schedule_exec (doc_id);

CREATE TABLE IF NOT EXISTS t_knowledge_chunk_vector (
    chunk_id    VARCHAR(20) NOT NULL PRIMARY KEY,
    doc_id      VARCHAR(20) NOT NULL,
    kb_id       VARCHAR(20) NOT NULL,
    chunk_index INTEGER NOT NULL,
    content     TEXT NOT NULL,
    embedding   VECTOR,
    metadata    JSONB,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_chunk_vector_doc_id ON t_knowledge_chunk_vector (doc_id);
CREATE INDEX IF NOT EXISTS idx_chunk_vector_kb_id ON t_knowledge_chunk_vector (kb_id);

CREATE TABLE IF NOT EXISTS t_user (
    id            VARCHAR(20)   NOT NULL PRIMARY KEY,
    username      VARCHAR(32)   NOT NULL,
    password_hash VARCHAR(128)  NOT NULL,
    role          VARCHAR(16)   NOT NULL,
    avatar        VARCHAR(1024),
    created_by    VARCHAR(32)   NOT NULL,
    updated_by    VARCHAR(32),
    create_time   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted       SMALLINT      NOT NULL DEFAULT 0,
    CONSTRAINT uk_user_username UNIQUE (username)
);
CREATE INDEX IF NOT EXISTS idx_user_role ON t_user (role);

CREATE TABLE IF NOT EXISTS t_user_session (
    token       VARCHAR(64) NOT NULL PRIMARY KEY,
    user_id     VARCHAR(20) NOT NULL,
    expire_time TIMESTAMP   NOT NULL,
    create_time TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_session_user_id ON t_user_session (user_id);
CREATE INDEX IF NOT EXISTS idx_user_session_expire_time ON t_user_session (expire_time);

INSERT INTO t_user (id, username, password_hash, role, avatar, created_by, updated_by, create_time, update_time, deleted)
VALUES ('1', 'admin', '$2a$10$cIjmSH.FjK9r5tqJlQLEIeKnz.tlHtf3xTnB7BssZYG/mX9J2Jy32', 'admin', '', 'system', 'system', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0)
ON CONFLICT (username) DO NOTHING;
