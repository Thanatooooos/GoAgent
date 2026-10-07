ALTER TABLE t_scheduled_task
    ADD COLUMN IF NOT EXISTS last_feedback_at TIMESTAMPTZ;
