ALTER TABLE t_conversation_summary
  ADD COLUMN IF NOT EXISTS structured_summary_json TEXT;
