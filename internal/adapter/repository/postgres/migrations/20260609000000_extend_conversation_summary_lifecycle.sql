ALTER TABLE t_conversation_summary
  ADD COLUMN IF NOT EXISTS summary_version INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS covered_from_message_id VARCHAR(20),
  ADD COLUMN IF NOT EXISTS covered_to_message_id VARCHAR(20),
  ADD COLUMN IF NOT EXISTS source_message_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS quality_status VARCHAR(32) NOT NULL DEFAULT 'unchecked',
  ADD COLUMN IF NOT EXISTS last_rebuild_reason TEXT;
