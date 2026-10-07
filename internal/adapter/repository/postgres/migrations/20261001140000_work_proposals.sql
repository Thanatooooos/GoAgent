CREATE TABLE t_work_proposal (
 id varchar(32) PRIMARY KEY,
 topic_id varchar(32) NOT NULL REFERENCES t_work_topic(id),
 turn_id varchar(32) NOT NULL REFERENCES t_work_turn(id),
 base_revision integer NOT NULL,
 changes_json jsonb NOT NULL,
 fingerprint varchar(64) NOT NULL,
 status varchar(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','ignored')),
 applied_revision integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(topic_id,fingerprint)
);
CREATE INDEX idx_work_proposal_topic ON t_work_proposal(topic_id,created_at DESC);
CREATE TABLE t_work_turn_operation (
 turn_id varchar(32) NOT NULL REFERENCES t_work_turn(id),
 kind varchar(32) NOT NULL,
 tool_call_id varchar(128) NOT NULL,
 input_hash varchar(64) NOT NULL,
 result_json jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(turn_id,kind)
);
