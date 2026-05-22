ALTER TABLE t_memory_item
    ADD COLUMN IF NOT EXISTS namespace VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS category VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS canonical_key VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS value_type VARCHAR(16) NOT NULL DEFAULT 'text',
    ADD COLUMN IF NOT EXISTS value_json TEXT,
    ADD COLUMN IF NOT EXISTS display_value TEXT,
    ADD COLUMN IF NOT EXISTS importance INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS supersedes_id VARCHAR(20),
    ADD COLUMN IF NOT EXISTS extraction_method VARCHAR(32) NOT NULL DEFAULT 'manual';

CREATE INDEX IF NOT EXISTS idx_memory_item_user_scope_key_status
    ON t_memory_item (user_id, scope_type, scope_id, canonical_key, status);

CREATE INDEX IF NOT EXISTS idx_memory_item_user_namespace_category_status
    ON t_memory_item (user_id, namespace, category, status);

CREATE INDEX IF NOT EXISTS idx_memory_item_supersedes_id
    ON t_memory_item (supersedes_id);
