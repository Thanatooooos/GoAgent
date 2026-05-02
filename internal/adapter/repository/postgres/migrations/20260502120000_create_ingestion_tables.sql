CREATE TABLE t_ingestion_pipeline (
    id          VARCHAR(20) NOT NULL PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    description VARCHAR(1024),
    nodes_json  JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by  VARCHAR(20) NOT NULL,
    updated_by  VARCHAR(20),
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted     SMALLINT DEFAULT 0
);
CREATE INDEX idx_ingestion_pipeline_created_by ON t_ingestion_pipeline (created_by);
CREATE INDEX idx_ingestion_pipeline_name ON t_ingestion_pipeline (name);

CREATE TABLE t_ingestion_task (
    id              VARCHAR(20) NOT NULL PRIMARY KEY,
    pipeline_id     VARCHAR(20) NOT NULL,
    source_type     VARCHAR(16) NOT NULL,
    source_location VARCHAR(1024),
    source_file_name VARCHAR(256),
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    chunk_count     INTEGER NOT NULL DEFAULT 0,
    error_message   VARCHAR(1000),
    metadata        JSONB,
    started_at      TIMESTAMP(3),
    completed_at    TIMESTAMP(3),
    created_by      VARCHAR(20) NOT NULL,
    updated_by      VARCHAR(20),
    create_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT DEFAULT 0
);
CREATE INDEX idx_ingestion_task_pipeline_id ON t_ingestion_task (pipeline_id);
CREATE INDEX idx_ingestion_task_status ON t_ingestion_task (status);
CREATE INDEX idx_ingestion_task_created_by ON t_ingestion_task (created_by);

CREATE TABLE t_ingestion_task_node (
    id            VARCHAR(20) NOT NULL PRIMARY KEY,
    task_id       VARCHAR(20) NOT NULL,
    pipeline_id   VARCHAR(20) NOT NULL,
    node_id       VARCHAR(64) NOT NULL,
    node_type     VARCHAR(16) NOT NULL,
    node_order    INTEGER DEFAULT 0,
    status        VARCHAR(16) NOT NULL DEFAULT 'pending',
    duration_ms   BIGINT,
    message       VARCHAR(1000),
    error_message VARCHAR(1000),
    output        JSONB,
    create_time   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted       SMALLINT DEFAULT 0,
    CONSTRAINT uk_ingestion_task_node UNIQUE (task_id, node_id)
);
CREATE INDEX idx_ingestion_task_node_task_id ON t_ingestion_task_node (task_id);
CREATE INDEX idx_ingestion_task_node_pipeline_id ON t_ingestion_task_node (pipeline_id);
CREATE INDEX idx_ingestion_task_node_task_order ON t_ingestion_task_node (task_id, node_order);
