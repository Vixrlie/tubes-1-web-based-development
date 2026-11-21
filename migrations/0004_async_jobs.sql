-- Asynchronous migration processing: persistent job queue, distributed per-database lock,
-- and per-batch checkpoints for resume and reverse-order compensating rollback.

CREATE TABLE migration_jobs (
    id                BIGSERIAL   PRIMARY KEY,
    migration_id      BIGINT      NOT NULL REFERENCES migration_history (id) ON DELETE CASCADE,
    execution_id      BIGINT,
    connection_id     BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    action            TEXT        NOT NULL CHECK (action IN ('up', 'down', 'forward', 'resume', 'cancel_batch')),
    mode              TEXT        NOT NULL DEFAULT 'normal' CHECK (mode IN ('normal', 'batch')),
    sql_up            TEXT        NOT NULL,
    sql_down          TEXT,
    irreversible      BOOLEAN     NOT NULL DEFAULT FALSE,
    batch_table       TEXT,
    batch_column      TEXT,
    batch_size        INT         CHECK (batch_size IS NULL OR batch_size > 0),
    cursor_min        TEXT,
    cursor_max        TEXT,
    last_cursor       TEXT,
    committed_batches INT         NOT NULL DEFAULT 0,
    committed_rows    BIGINT      NOT NULL DEFAULT 0,
    status            TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'running',
        'success',
        'failed',
        'partial',
        'partially_reverted',
        'reverted'
    )),
    parent_job_id     BIGINT      REFERENCES migration_jobs (id) ON DELETE SET NULL,
    idempotency_key   TEXT        UNIQUE,
    attempt           INT         NOT NULL DEFAULT 0,
    max_retries       INT         NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
    next_retry_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    trace_context     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    error             TEXT,
    created_by        BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX migration_jobs_dequeue_idx
    ON migration_jobs (next_retry_at, id)
    WHERE status = 'pending';

CREATE INDEX migration_jobs_connection_status_idx
    ON migration_jobs (connection_id, status);

CREATE INDEX migration_jobs_migration_idx
    ON migration_jobs (migration_id);

-- Distributed lock per target database (berlaku lintas worker & bertahan antar commit batch).
CREATE TABLE migration_locks (
    connection_id  BIGINT      PRIMARY KEY REFERENCES db_connections (id) ON DELETE CASCADE,
    job_id         BIGINT      NOT NULL REFERENCES migration_jobs (id) ON DELETE CASCADE,
    worker_id      TEXT        NOT NULL,
    locked_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX migration_locks_lease_idx
    ON migration_locks (lease_expires_at);

-- Checkpoint per batch untuk resume dan pembatalan (compensating rollback) urutan terbalik.
CREATE TABLE migration_batch_checkpoints (
    id            BIGSERIAL   PRIMARY KEY,
    job_id        BIGINT      NOT NULL REFERENCES migration_jobs (id) ON DELETE CASCADE,
    batch_number  INT         NOT NULL CHECK (batch_number > 0),
    cursor_start  TEXT        NOT NULL,
    cursor_end    TEXT        NOT NULL,
    rows_affected BIGINT      NOT NULL DEFAULT 0,
    status        TEXT        NOT NULL DEFAULT 'committed' CHECK (status IN ('committed', 'reverted')),
    recovery_data JSONB,
    committed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reverted_at   TIMESTAMPTZ,
    UNIQUE (job_id, batch_number)
);

CREATE INDEX migration_batch_checkpoints_job_status_idx
    ON migration_batch_checkpoints (job_id, status, batch_number DESC);
