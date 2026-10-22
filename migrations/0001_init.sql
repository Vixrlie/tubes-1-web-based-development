-- Initial Schema
-- Platform schema: users, groups, memberships, grants, sessions,
-- db connections, and migration history.

CREATE TABLE users (
    id                BIGSERIAL   PRIMARY KEY,
    username          TEXT        NOT NULL UNIQUE,
    full_name         TEXT,
    email             TEXT        UNIQUE,
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
    group_id      BIGINT      NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    connection_id BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    level         TEXT        NOT NULL CHECK (level IN ('view', 'edit')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, connection_id)
);

CREATE INDEX grants_connection_idx ON grants (connection_id);

CREATE TABLE sessions (
    id                   BIGSERIAL   PRIMARY KEY,
    token_hash           TEXT        NOT NULL UNIQUE,
    previous_token_hash  TEXT,
    previous_valid_until TIMESTAMPTZ,
    user_id              BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf_secret          TEXT        NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    absolute_expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at           TIMESTAMPTZ
);

CREATE INDEX sessions_user_idx           ON sessions (user_id);
CREATE INDEX sessions_previous_token_idx ON sessions (previous_token_hash);
CREATE INDEX sessions_abs_expires_idx    ON sessions (absolute_expires_at);

CREATE TABLE migration_history (
    id            BIGSERIAL   PRIMARY KEY,
    connection_id BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    version       INTEGER     NOT NULL,
    sql           TEXT        NOT NULL,
    checksum      TEXT        NOT NULL,
    status        TEXT        NOT NULL CHECK (status IN ('draft', 'applied', 'failed')),
    error         TEXT,
    created_by    BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    executed_by   BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    executed_at   TIMESTAMPTZ,
    UNIQUE (connection_id, version)
);

CREATE INDEX migration_history_connection_idx ON migration_history (connection_id);
