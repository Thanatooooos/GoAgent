-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS t_user (
    id            VARCHAR(20)   NOT NULL PRIMARY KEY,
    username      VARCHAR(32)   NOT NULL,
    password_hash VARCHAR(128)  NOT NULL,
    role          VARCHAR(16)   NOT NULL,
    avatar        VARCHAR(1024),
    created_by    VARCHAR(32)   NOT NULL,
    updated_by    VARCHAR(32),
    create_time   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted       SMALLINT      NOT NULL DEFAULT 0,
    CONSTRAINT uk_user_username UNIQUE (username)
);
CREATE INDEX IF NOT EXISTS idx_user_role ON t_user (role);

CREATE TABLE IF NOT EXISTS t_user_session (
    token       VARCHAR(64) NOT NULL PRIMARY KEY,
    user_id     VARCHAR(20) NOT NULL,
    expire_time TIMESTAMP   NOT NULL,
    create_time TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_session_user_id ON t_user_session (user_id);
CREATE INDEX IF NOT EXISTS idx_user_session_expire_time ON t_user_session (expire_time);

INSERT INTO t_user (id, username, password_hash, role, avatar, created_by, updated_by, create_time, update_time, deleted)
VALUES ('1', 'admin', '$2a$10$cIjmSH.FjK9r5tqJlQLEIeKnz.tlHtf3xTnB7BssZYG/mX9J2Jy32', 'admin', '', 'system', 'system', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0)
ON CONFLICT (username) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- DROP TABLE IF EXISTS t_user_session;
-- DROP TABLE IF EXISTS t_user;
-- +goose StatementEnd
