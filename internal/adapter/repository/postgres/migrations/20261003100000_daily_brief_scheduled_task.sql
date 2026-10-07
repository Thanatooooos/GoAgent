ALTER TABLE t_scheduled_task_version ADD COLUMN IF NOT EXISTS daily_brief_json JSONB;

-- Empty on installation: deployment alone never transfers a subscription.
CREATE TABLE IF NOT EXISTS t_daily_brief_task_binding (
    user_id VARCHAR(20) PRIMARY KEY REFERENCES t_daily_brief_subscription(user_id),
    task_id VARCHAR(64) NOT NULL UNIQUE REFERENCES t_scheduled_task(id),
    cutover_at TIMESTAMPTZ NOT NULL
);

-- Fence old binaries too. Returning NULL makes a legacy lease acquisition a
-- zero-row update. Old subscription writers cannot silently desync the task.
CREATE OR REPLACE FUNCTION guard_daily_brief_subscription() RETURNS trigger AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM t_daily_brief_task_binding WHERE user_id = NEW.user_id) THEN
        IF NEW.lock_owner IS NOT NULL THEN RETURN NULL; END IF;
        IF (NEW.enabled, NEW.timezone, NEW.delivery_time_local, NEW.topics_json, NEW.sources_json)
           IS DISTINCT FROM (OLD.enabled, OLD.timezone, OLD.delivery_time_local, OLD.topics_json, OLD.sources_json)
           AND current_setting('app.daily_brief_sync', true) IS DISTINCT FROM 'on' THEN
            RAISE EXCEPTION 'DailyBrief subscription requires the scheduled-task writer';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER daily_brief_subscription_owner BEFORE UPDATE ON t_daily_brief_subscription
FOR EACH ROW EXECUTE FUNCTION guard_daily_brief_subscription();

-- Serialize publication with handoff, including a late legacy execution whose
-- lease expired. Only a bound occurrence can write an issue after handoff.
CREATE OR REPLACE FUNCTION guard_daily_brief_issue() RETURNS trigger AS $$
DECLARE bound_task VARCHAR(64);
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('daily-brief:' || NEW.user_id));
    SELECT task_id INTO bound_task FROM t_daily_brief_task_binding WHERE user_id = NEW.user_id;
    IF bound_task IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM t_scheduled_task_occurrence o
        WHERE o.id = NEW.published_run_id AND o.task_id = bound_task
    ) THEN
        RAISE EXCEPTION 'DailyBrief legacy issue writer is fenced';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status = 'ready' AND
       (NEW.status, NEW.published_run_id, NEW.headline, NEW.top_summary, NEW.sections_json)
       IS DISTINCT FROM (OLD.status, OLD.published_run_id, OLD.headline, OLD.top_summary, OLD.sections_json) THEN
        RAISE EXCEPTION 'DailyBrief published date is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER daily_brief_issue_owner BEFORE INSERT OR UPDATE ON t_daily_brief_issue
FOR EACH ROW EXECUTE FUNCTION guard_daily_brief_issue();

-- Deferred until the new immutable version exists. Covers generic edits,
-- pause/resume/delete and conversation deletion, including existing SQL paths.
CREATE OR REPLACE FUNCTION sync_daily_brief_task_subscription() RETURNS trigger AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM t_daily_brief_task_binding WHERE task_id=NEW.id) THEN
        PERFORM set_config('app.daily_brief_sync','on',true);
        UPDATE t_daily_brief_subscription s SET
            enabled=CASE WHEN t.status='active' AND t.deleted_at IS NULL THEN 1 ELSE 0 END,
            timezone=v.schedule_json->>'timezone',delivery_time_local=v.schedule_json->>'localTime',
            topics_json=v.daily_brief_json->'topics',sources_json=v.daily_brief_json->'sources',
            lock_owner=NULL,lock_until=NULL
        FROM t_scheduled_task t JOIN t_scheduled_task_version v ON v.task_id=t.id AND v.version=t.current_version
        WHERE t.id=NEW.id AND s.user_id=t.user_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER daily_brief_task_subscription AFTER UPDATE ON t_scheduled_task
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sync_daily_brief_task_subscription();

CREATE OR REPLACE FUNCTION guard_daily_brief_task_version() RETURNS trigger AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM t_daily_brief_task_binding WHERE task_id=NEW.task_id) AND
       (NEW.daily_brief_json IS NULL OR NEW.schedule_json->>'kind' IS DISTINCT FROM 'daily' OR NEW.report_mode <> 'always') THEN
        RAISE EXCEPTION 'DailyBrief output contract cannot be removed';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER daily_brief_task_version BEFORE INSERT ON t_scheduled_task_version
FOR EACH ROW EXECUTE FUNCTION guard_daily_brief_task_version();
