CREATE TABLE t_work_turn (
    id VARCHAR(20) PRIMARY KEY,
    topic_id VARCHAR(20) NOT NULL REFERENCES t_work_topic(id),
    user_id VARCHAR(20) NOT NULL,
    client_request_id VARCHAR(128) NOT NULL,
    conversation_id VARCHAR(20) NOT NULL,
    item_id VARCHAR(20),
    artifact_id VARCHAR(20),
    artifact_revision INTEGER NOT NULL DEFAULT 0,
    state_revision INTEGER NOT NULL,
    user_message_id VARCHAR(20) NOT NULL,
    assistant_message_id VARCHAR(20),
    question TEXT NOT NULL,
    action VARCHAR(32) NOT NULL DEFAULT 'discuss',
    input_hash VARCHAR(64) NOT NULL,
    context_json JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'accepted',
    error TEXT NOT NULL DEFAULT '',
    deadline_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id,client_request_id),
    FOREIGN KEY (topic_id,conversation_id) REFERENCES t_work_conversation(topic_id,conversation_id),
    FOREIGN KEY (topic_id,item_id) REFERENCES t_work_item(topic_id,id),
    FOREIGN KEY (topic_id,artifact_id) REFERENCES t_work_artifact(topic_id,id)
);
CREATE INDEX idx_work_turn_conversation ON t_work_turn(topic_id,conversation_id,created_at DESC);
CREATE INDEX idx_work_turn_pending ON t_work_turn(status,deadline_at);
