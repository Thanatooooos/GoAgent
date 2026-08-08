-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS t_daily_brief_subscription (
    user_id              VARCHAR(20)  NOT NULL PRIMARY KEY,
    enabled              SMALLINT     NOT NULL DEFAULT 1,
    timezone             VARCHAR(64)  NOT NULL,
    delivery_time_local  VARCHAR(8)   NOT NULL,
    topics_json          JSONB        NOT NULL DEFAULT '[]'::jsonb,
    sources_json         JSONB        NOT NULL DEFAULT '[]'::jsonb,
    lock_owner           VARCHAR(128),
    lock_until           TIMESTAMP,
    create_time          TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time          TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_daily_brief_subscription_enabled ON t_daily_brief_subscription (enabled);
CREATE INDEX IF NOT EXISTS idx_daily_brief_subscription_lock_until ON t_daily_brief_subscription (lock_until);

CREATE TABLE IF NOT EXISTS t_daily_brief_issue (
    id                   VARCHAR(20)   NOT NULL PRIMARY KEY,
    user_id              VARCHAR(20)   NOT NULL,
    brief_date           VARCHAR(10)   NOT NULL,
    status               VARCHAR(16)   NOT NULL,
    headline             VARCHAR(512),
    top_summary          TEXT,
    sections_json        JSONB         NOT NULL DEFAULT '[]'::jsonb,
    item_count           INTEGER       NOT NULL DEFAULT 0,
    published_run_id     VARCHAR(20),
    generated_at         TIMESTAMP,
    published_at         TIMESTAMP,
    create_time          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_daily_brief_issue_user_date UNIQUE (user_id, brief_date)
);
CREATE INDEX IF NOT EXISTS idx_daily_brief_issue_user_status ON t_daily_brief_issue (user_id, status);

CREATE TABLE IF NOT EXISTS t_daily_brief_item (
    id                   VARCHAR(20)   NOT NULL PRIMARY KEY,
    issue_id             VARCHAR(20)   NOT NULL,
    section_key          VARCHAR(64)   NOT NULL,
    rank                 INTEGER       NOT NULL,
    title                VARCHAR(512)  NOT NULL,
    summary              TEXT,
    why_it_matters       TEXT,
    url                  VARCHAR(2048),
    source               VARCHAR(64),
    topic                VARCHAR(64),
    published_at         TIMESTAMP,
    metadata_json        JSONB         NOT NULL DEFAULT '{}'::jsonb,
    create_time          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_daily_brief_item_issue_section_rank ON t_daily_brief_item (issue_id, section_key, rank);

CREATE TABLE IF NOT EXISTS t_daily_brief_generation_run (
    id                   VARCHAR(20)   NOT NULL PRIMARY KEY,
    user_id              VARCHAR(20)   NOT NULL,
    brief_date           VARCHAR(10)   NOT NULL,
    trigger_type         VARCHAR(16)   NOT NULL,
    status               VARCHAR(16)   NOT NULL,
    started_at           TIMESTAMP     NOT NULL,
    finished_at          TIMESTAMP,
    error_message        VARCHAR(1024),
    source_stats_json    JSONB         NOT NULL DEFAULT '{}'::jsonb,
    model                VARCHAR(128),
    prompt_version       VARCHAR(64),
    token_usage_json     JSONB         NOT NULL DEFAULT '{}'::jsonb,
    create_time          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_daily_brief_generation_run_user_date_started ON t_daily_brief_generation_run (user_id, brief_date, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_daily_brief_generation_run_status_finished ON t_daily_brief_generation_run (status, finished_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- DROP TABLE IF EXISTS t_daily_brief_generation_run;
-- DROP TABLE IF EXISTS t_daily_brief_item;
-- DROP TABLE IF EXISTS t_daily_brief_issue;
-- DROP TABLE IF EXISTS t_daily_brief_subscription;
-- +goose StatementEnd
