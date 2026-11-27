-- Milestone 2: migration dengan SQL up/down, workflow approval per leader
-- group, riwayat eksekusi append-only (rollback/forward/gagal ikut tercatat),
-- dan antrian job untuk eksekusi asynchronous.
--
-- Tabel M1 migration_history (satu baris = definisi + hasil eksekusi) dipecah:
--   migrations                  definisi (SQL up/down), immutable setelah dieksekusi
--   migration_requests          pengajuan up/rollback/forward yang butuh approval
--   migration_request_reviews   keputusan leader per group
--   migration_executions        setiap attempt eksekusi (riwayat migration)
--   migration_jobs              antrian yang diambil worker

-- Leader group: satu-satunya anggota yang boleh memberi approval atas nama group.
ALTER TABLE groups ADD COLUMN leader_id BIGINT REFERENCES users (id) ON DELETE SET NULL;

-- Leader yang dikeluarkan dari group otomatis dilepas, jadi leader selalu anggota.
CREATE FUNCTION groups_clear_removed_leader() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE groups SET leader_id = NULL WHERE id = OLD.group_id AND leader_id = OLD.user_id;
    RETURN OLD;
END;
$$;

CREATE TRIGGER memberships_clear_leader
    AFTER DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION groups_clear_removed_leader();

CREATE TABLE migrations (
    id                  BIGSERIAL   PRIMARY KEY,
    connection_id       BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    version             BIGINT      NOT NULL,
    name                TEXT        NOT NULL,
    up_sql              TEXT        NOT NULL,
    -- down_sql NULL berarti irreversible; alasannya wajib diisi.
    down_sql            TEXT,
    irreversible_reason TEXT,
    revision            INT         NOT NULL DEFAULT 1,
    checksum            TEXT        NOT NULL,
    -- Diisi saat eksekusi pertama; sejak itu SQL up/down tidak boleh berubah.
    locked_at           TIMESTAMPTZ,
    validation_ok       BOOLEAN,
    validation_message  TEXT,
    validated_checksum  TEXT,
    validated_at        TIMESTAMPTZ,
    created_by          BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (connection_id, version),
    CONSTRAINT migrations_reversibility CHECK (
        (down_sql IS NOT NULL AND btrim(down_sql) <> '' AND irreversible_reason IS NULL)
        OR (down_sql IS NULL AND irreversible_reason IS NOT NULL AND btrim(irreversible_reason) <> '')
    )
);

CREATE INDEX migrations_connection_idx ON migrations (connection_id);

CREATE FUNCTION migrations_guard_immutable() RETURNS trigger AS $$
BEGIN
    IF OLD.locked_at IS NOT NULL AND (
           NEW.up_sql              IS DISTINCT FROM OLD.up_sql
        OR NEW.down_sql            IS DISTINCT FROM OLD.down_sql
        OR NEW.irreversible_reason IS DISTINCT FROM OLD.irreversible_reason
        OR NEW.checksum            IS DISTINCT FROM OLD.checksum
        OR NEW.revision            IS DISTINCT FROM OLD.revision
        OR NEW.version             IS DISTINCT FROM OLD.version
        OR NEW.connection_id       IS DISTINCT FROM OLD.connection_id
        OR NEW.locked_at           IS NULL
    ) THEN
        RAISE EXCEPTION 'migration % sudah pernah dieksekusi; definisi SQL bersifat immutable', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER migrations_immutable
    BEFORE UPDATE ON migrations
    FOR EACH ROW EXECUTE FUNCTION migrations_guard_immutable();

CREATE TABLE migration_requests (
    id                  BIGSERIAL   PRIMARY KEY,
    migration_id        BIGINT      NOT NULL REFERENCES migrations (id) ON DELETE CASCADE,
    connection_id       BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    action              TEXT        NOT NULL CHECK (action IN ('up', 'rollback', 'forward')),
    -- Rollback: eksekusi up/forward yang akan dibatalkan.
    target_execution_id BIGINT,
    -- Salinan revisi dan checksum saat diajukan; dicek ulang saat approve dan eksekusi.
    migration_revision  INT         NOT NULL,
    checksum            TEXT        NOT NULL,
    status              TEXT        NOT NULL CHECK (status IN (
                            'pending_review', 'rejected', 'approved', 'queued',
                            'running', 'success', 'failed', 'cancelled')),
    note                TEXT,
    error               TEXT,
    requested_by        BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at          TIMESTAMPTZ,
    queued_at           TIMESTAMPTZ,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX migration_requests_connection_idx ON migration_requests (connection_id, status);
-- Satu migration hanya boleh punya satu pengajuan yang masih berjalan.
CREATE UNIQUE INDEX migration_requests_one_open
    ON migration_requests (migration_id)
 WHERE status IN ('pending_review', 'approved', 'queued', 'running');

CREATE TABLE migration_request_reviews (
    request_id BIGINT      NOT NULL REFERENCES migration_requests (id) ON DELETE CASCADE,
    group_id   BIGINT      NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    decision   TEXT        NOT NULL CHECK (decision IN ('approved', 'rejected')),
    note       TEXT,
    decided_by BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (request_id, group_id)
);

CREATE TABLE migration_executions (
    id                   BIGSERIAL   PRIMARY KEY,
    request_id           BIGINT      NOT NULL REFERENCES migration_requests (id) ON DELETE CASCADE,
    migration_id         BIGINT      NOT NULL REFERENCES migrations (id) ON DELETE CASCADE,
    connection_id        BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    action               TEXT        NOT NULL CHECK (action IN ('up', 'rollback', 'forward')),
    reverts_execution_id BIGINT      REFERENCES migration_executions (id) ON DELETE CASCADE,
    migration_version    BIGINT      NOT NULL,
    migration_revision   INT         NOT NULL,
    -- Salinan SQL yang benar-benar dijalankan, bukan referensi ke definisi.
    sql_executed         TEXT        NOT NULL,
    status               TEXT        NOT NULL CHECK (status IN ('running', 'success', 'failed')),
    -- revalidation | execution | platform
    error_stage          TEXT,
    error                TEXT,
    sqlstate             TEXT,
    attempt              INT         NOT NULL DEFAULT 1,
    job_id               BIGINT,
    -- txid transaksi di database target (PostgreSQL). Bila worker mati sebelum
    -- hasil tercatat, txid_status() di target menjadi bukti commit/abort.
    target_txid          BIGINT,
    executed_by          BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at          TIMESTAMPTZ,
    duration_ms          BIGINT,
    rows_affected        BIGINT
);

CREATE INDEX migration_executions_connection_idx ON migration_executions (connection_id, id DESC);
CREATE INDEX migration_executions_migration_idx  ON migration_executions (migration_id, id DESC);
-- Satu eksekusi hanya dapat dibatalkan oleh satu rollback yang berhasil.
CREATE UNIQUE INDEX migration_executions_single_revert
    ON migration_executions (reverts_execution_id)
 WHERE status = 'success';

ALTER TABLE migration_requests
    ADD CONSTRAINT migration_requests_target_execution_fk
    FOREIGN KEY (target_execution_id) REFERENCES migration_executions (id) ON DELETE CASCADE;

-- Riwayat bersifat append-only: baris yang sudah final (success/failed) tidak
-- boleh ditimpa. Hanya executed_by yang boleh menjadi NULL karena FK.
CREATE FUNCTION migration_executions_append_only() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'running' AND (
           NEW.status        IS DISTINCT FROM OLD.status
        OR NEW.sql_executed  IS DISTINCT FROM OLD.sql_executed
        OR NEW.error         IS DISTINCT FROM OLD.error
        OR NEW.error_stage   IS DISTINCT FROM OLD.error_stage
        OR NEW.sqlstate      IS DISTINCT FROM OLD.sqlstate
        OR NEW.action        IS DISTINCT FROM OLD.action
        OR NEW.migration_id  IS DISTINCT FROM OLD.migration_id
        OR NEW.request_id    IS DISTINCT FROM OLD.request_id
        OR NEW.reverts_execution_id IS DISTINCT FROM OLD.reverts_execution_id
        OR NEW.started_at    IS DISTINCT FROM OLD.started_at
        OR NEW.finished_at   IS DISTINCT FROM OLD.finished_at
        OR NEW.rows_affected IS DISTINCT FROM OLD.rows_affected
    ) THEN
        RAISE EXCEPTION 'riwayat eksekusi % sudah final dan tidak boleh ditimpa', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.status = 'running' AND (
           NEW.sql_executed IS DISTINCT FROM OLD.sql_executed
        OR NEW.migration_id IS DISTINCT FROM OLD.migration_id
        OR NEW.action       IS DISTINCT FROM OLD.action
    ) THEN
        RAISE EXCEPTION 'salinan SQL eksekusi % tidak boleh diubah', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER migration_executions_append_only
    BEFORE UPDATE ON migration_executions
    FOR EACH ROW EXECUTE FUNCTION migration_executions_append_only();

-- Antrian job. Implementasi worker sementara memakai SELECT ... FOR UPDATE
-- SKIP LOCKED; satu request hanya punya satu job sehingga pesan duplikat
-- tidak membuat eksekusi ganda.
CREATE TABLE migration_jobs (
    id            BIGSERIAL   PRIMARY KEY,
    request_id    BIGINT      NOT NULL UNIQUE REFERENCES migration_requests (id) ON DELETE CASCADE,
    connection_id BIGINT      NOT NULL REFERENCES db_connections (id) ON DELETE CASCADE,
    status        TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'running', 'done', 'failed')),
    attempts      INT         NOT NULL DEFAULT 0,
    max_attempts  INT         NOT NULL DEFAULT 3,
    available_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_by     TEXT,
    locked_at     TIMESTAMPTZ,
    last_error    TEXT,
    -- W3C trace context dari request API, diteruskan ke span worker.
    trace_context JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX migration_jobs_pick_idx ON migration_jobs (status, available_at);

-- Pindahkan data M1. Migration M1 hanya punya satu SQL, jadi ditandai
-- irreversible; draft tetap draft dan bisa dilengkapi SQL down sebelum diajukan.
INSERT INTO migrations (id, connection_id, version, name, up_sql, down_sql,
                        irreversible_reason, revision, checksum, locked_at,
                        created_by, created_at, updated_at)
SELECT h.id, h.connection_id, h.version,
       COALESCE(
           NULLIF(btrim(substring(h.sql FROM '^\s*--\s*([^\n]*)')), ''),
           left(regexp_replace(btrim(h.sql), '\s+', ' ', 'g'), 80)),
       h.sql, NULL,
       'Migration Milestone 1 tanpa SQL down',
       1,
       encode(sha256(convert_to(h.sql || chr(30) || chr(30) || 'Migration Milestone 1 tanpa SQL down', 'UTF8')), 'hex'),
       CASE WHEN h.status <> 'draft' THEN COALESCE(h.executed_at, h.created_at) END,
       h.created_by, h.created_at, h.created_at
  FROM migration_history h;

SELECT setval(pg_get_serial_sequence('migrations', 'id'),
              GREATEST((SELECT MAX(id) FROM migrations), 1));

INSERT INTO migration_requests (migration_id, connection_id, action, migration_revision,
                                checksum, status, error, requested_by, created_at,
                                decided_at, queued_at, started_at, finished_at, updated_at)
SELECT m.id, m.connection_id, 'up', 1, m.checksum,
       CASE WHEN h.status = 'applied' THEN 'success' ELSE 'failed' END,
       h.error, h.executed_by, h.created_at,
       h.executed_at, h.executed_at, h.executed_at, h.executed_at, COALESCE(h.executed_at, h.created_at)
  FROM migration_history h
  JOIN migrations m ON m.id = h.id
 WHERE h.status IN ('applied', 'failed');

INSERT INTO migration_executions (request_id, migration_id, connection_id, action,
                                  migration_version, migration_revision, sql_executed,
                                  status, error_stage, error, executed_by,
                                  started_at, finished_at)
SELECT r.id, m.id, m.connection_id, 'up', m.version, 1, m.up_sql,
       CASE WHEN h.status = 'applied' THEN 'success' ELSE 'failed' END,
       CASE WHEN h.status = 'failed' THEN 'execution' END,
       h.error, h.executed_by,
       COALESCE(h.executed_at, h.created_at), COALESCE(h.executed_at, h.created_at)
  FROM migration_history h
  JOIN migrations m ON m.id = h.id
  JOIN migration_requests r ON r.migration_id = m.id
 WHERE h.status IN ('applied', 'failed');

DROP TABLE migration_history;
