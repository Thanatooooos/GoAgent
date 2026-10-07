ALTER TABLE t_scheduled_task ADD COLUMN IF NOT EXISTS last_status_scan_at TIMESTAMPTZ;
