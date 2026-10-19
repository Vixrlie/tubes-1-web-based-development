-- Initial Schema
-- Platform schema: users, groups, memberships, grants, sessions,
-- db connections, and migration history.

CREATE TABLE users (
    id                BIGSERIAL PRIMARY KEY,
    email             TEXT        NOT NULL UNIQUE,
    password_hash     TEXT        NOT NULL,
    is_platform_admin BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE groups (
    id         BIGSERIAL   PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    group_id   BIGINT      NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX memberships_group_idx ON memberships (group_id);

CREATE TABLE db_connections (
    id             BIGSERIAL   PRIMARY KEY,
    name           TEXT        NOT NULL UNIQUE,
    description    TEXT,
    connection_url TEXT        NOT NULL,
    created_by     BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE grants (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       BIGINT      REFERENCES users (id) ON DELETE CASCADE,
    group_id      BIGINT      REFERENCES groups (id) ON DELETE CASCADE,
    connection_id BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    permission    TEXT        NOT NULL CHECK (permission IN ('view', 'query', 'migrate')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(user_id, group_id) = 1)
);

CREATE UNIQUE INDEX grants_user_unique
    ON grants (user_id, connection_id, permission)
    WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX grants_group_unique
    ON grants (group_id, connection_id, permission)
    WHERE group_id IS NOT NULL;

CREATE INDEX grants_connection_idx ON grants (connection_id);

CREATE TABLE sessions (
    id           BIGSERIAL   PRIMARY KEY,
    token_hash   TEXT        NOT NULL UNIQUE,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE migration_history (
    id            BIGSERIAL   PRIMARY KEY,
    connection_id BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    version       TEXT        NOT NULL,
    checksum      TEXT        NOT NULL,
    status        TEXT        NOT NULL CHECK (status IN ('pending', 'applied', 'failed')),
    error         TEXT,
    applied_by    BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    applied_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (connection_id, version)
);

CREATE INDEX migration_history_connection_idx ON migration_history (connection_id);
