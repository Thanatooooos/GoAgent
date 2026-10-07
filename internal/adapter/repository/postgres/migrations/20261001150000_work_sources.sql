ALTER TABLE t_knowledge_base ADD COLUMN work_private boolean NOT NULL DEFAULT false;
CREATE TABLE t_work_source (
 id varchar(32) PRIMARY KEY,
 topic_id varchar(32) NOT NULL REFERENCES t_work_topic(id),
 user_id varchar(20) NOT NULL,
 conversation_id varchar(20),
 kb_id varchar(20) NOT NULL REFERENCES t_knowledge_base(id),
 doc_id varchar(20),
 name varchar(256) NOT NULL,
 source_type varchar(16) NOT NULL,
 source_location text NOT NULL DEFAULT '',
 promoted boolean NOT NULL DEFAULT false,
 status varchar(24) NOT NULL DEFAULT 'reserved',
 file_key text NOT NULL DEFAULT '',
 error text NOT NULL DEFAULT '',
 cleanup_attempts integer NOT NULL DEFAULT 0,
 next_cleanup_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 FOREIGN KEY(topic_id,conversation_id) REFERENCES t_work_conversation(topic_id,conversation_id)
);
CREATE INDEX idx_work_source_scope ON t_work_source(topic_id,status,conversation_id);
