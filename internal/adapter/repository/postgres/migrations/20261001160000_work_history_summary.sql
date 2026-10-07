CREATE TABLE t_work_history_summary (
 topic_id varchar(32) NOT NULL REFERENCES t_work_topic(id),
 conversation_id varchar(20) NOT NULL,
 item_key varchar(32) NOT NULL DEFAULT '',
 content text NOT NULL,
 structured_json text NOT NULL,
 covered_from_message_id varchar(20) NOT NULL,
 covered_to_message_id varchar(20) NOT NULL,
 source_message_count integer NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(conversation_id,item_key),
 FOREIGN KEY(topic_id,conversation_id) REFERENCES t_work_conversation(topic_id,conversation_id)
);
