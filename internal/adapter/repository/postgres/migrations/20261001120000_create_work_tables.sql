CREATE TABLE t_work_topic (
    id VARCHAR(20) PRIMARY KEY,
    user_id VARCHAR(20) NOT NULL,
    name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_work_topic_owner ON t_work_topic(user_id,status,updated_at DESC,id);
CREATE TABLE t_work_item (
    id VARCHAR(20) PRIMARY KEY,
    topic_id VARCHAR(20) NOT NULL REFERENCES t_work_topic(id),
    name VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (topic_id,id)
);
CREATE TABLE t_work_conversation (
    conversation_id VARCHAR(20) PRIMARY KEY,
    topic_id VARCHAR(20) NOT NULL REFERENCES t_work_topic(id),
    user_id VARCHAR(20) NOT NULL,
    item_id VARCHAR(20),
    continue_from VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (topic_id,item_id) REFERENCES t_work_item(topic_id,id),
    UNIQUE (topic_id,conversation_id)
);
CREATE INDEX idx_work_conversation_topic ON t_work_conversation(topic_id,created_at DESC);
CREATE TABLE t_work_state (
    topic_id VARCHAR(20) PRIMARY KEY REFERENCES t_work_topic(id),
    revision INTEGER NOT NULL CHECK (revision > 0)
);
CREATE TABLE t_work_state_revision (
    topic_id VARCHAR(20) NOT NULL REFERENCES t_work_topic(id),
    revision INTEGER NOT NULL,
    entries_json JSONB NOT NULL,
    author VARCHAR(16) NOT NULL CHECK (author IN ('user','ai')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (topic_id,revision)
);
CREATE TABLE t_work_artifact (
    id VARCHAR(20) PRIMARY KEY,
    topic_id VARCHAR(20) NOT NULL REFERENCES t_work_topic(id),
    item_id VARCHAR(20),
    title VARCHAR(128) NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (topic_id,item_id) REFERENCES t_work_item(topic_id,id),
    UNIQUE (topic_id,id)
);
CREATE INDEX idx_work_artifact_topic ON t_work_artifact(topic_id,updated_at DESC,id);
CREATE TABLE t_work_artifact_version (
    artifact_id VARCHAR(20) NOT NULL REFERENCES t_work_artifact(id),
    revision INTEGER NOT NULL,
    title VARCHAR(128) NOT NULL,
    body_json JSONB NOT NULL,
    author VARCHAR(16) NOT NULL CHECK (author IN ('user','ai')),
    summary TEXT NOT NULL DEFAULT '',
    conversation_id VARCHAR(20),
    restored_from INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (artifact_id,revision)
);
CREATE TABLE t_work_operation (
    user_id VARCHAR(20) NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    topic_id VARCHAR(20) NOT NULL REFERENCES t_work_topic(id),
    kind VARCHAR(40) NOT NULL,
    input_hash VARCHAR(64) NOT NULL,
    result_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id,request_id)
);
