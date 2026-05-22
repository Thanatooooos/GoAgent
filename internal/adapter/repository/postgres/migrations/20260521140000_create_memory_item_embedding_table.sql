CREATE TABLE IF NOT EXISTS t_memory_item_embedding (
    memory_item_id VARCHAR(20) NOT NULL PRIMARY KEY,
    embedding      VECTOR NOT NULL,
    create_time    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memory_item_embedding_memory_item
    ON t_memory_item_embedding (memory_item_id);
