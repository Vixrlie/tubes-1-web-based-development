-- Milestone 2: migration per batch.
--
-- Mode batch adalah bagian dari definisi migration (tabel, kolom cursor, dan
-- ukuran batch), sehingga ikut checksum, approval, dan aturan immutable.
-- SQL up/down mode batch memakai placeholder $1 (awal) dan $2 (akhir) rentang
-- cursor, misalnya: UPDATE t SET x = 1 WHERE id BETWEEN $1 AND $2.
--
-- Setiap batch di-commit dalam transaksinya sendiri bersama checkpoint di
-- database target (_pathgres_batch_checkpoints). Salinan checkpoint di sini
-- hanya untuk tampilan; database target tetap sumber kebenaran.

ALTER TABLE migrations
    ADD COLUMN batch_table  TEXT,
    ADD COLUMN batch_column TEXT,
    ADD COLUMN batch_size   INT,
    ADD CONSTRAINT migrations_batch_config CHECK (
        (batch_table IS NULL AND batch_column IS NULL AND batch_size IS NULL)
        OR (batch_table IS NOT NULL AND batch_column IS NOT NULL AND batch_size > 0)
    );

CREATE OR REPLACE FUNCTION migrations_guard_immutable() RETURNS trigger AS $$
BEGIN
    IF OLD.locked_at IS NOT NULL AND (
           NEW.up_sql              IS DISTINCT FROM OLD.up_sql
        OR NEW.down_sql            IS DISTINCT FROM OLD.down_sql
        OR NEW.irreversible_reason IS DISTINCT FROM OLD.irreversible_reason
        OR NEW.checksum            IS DISTINCT FROM OLD.checksum
        OR NEW.revision            IS DISTINCT FROM OLD.revision
        OR NEW.version             IS DISTINCT FROM OLD.version
        OR NEW.connection_id       IS DISTINCT FROM OLD.connection_id
        OR NEW.batch_table         IS DISTINCT FROM OLD.batch_table
        OR NEW.batch_column        IS DISTINCT FROM OLD.batch_column
        OR NEW.batch_size          IS DISTINCT FROM OLD.batch_size
        OR NEW.locked_at           IS NULL
    ) THEN
        RAISE EXCEPTION 'migration % sudah pernah dieksekusi; definisi SQL bersifat immutable', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- resume: melanjutkan eksekusi batch yang berhenti sebagian dari checkpoint.
-- partial: eksekusi batch berhenti setelah sebagian batch committed; database
-- ditahan sampai dilanjutkan (resume) atau dibatalkan (rollback per batch).
ALTER TABLE migration_requests DROP CONSTRAINT migration_requests_action_check;
ALTER TABLE migration_requests ADD CONSTRAINT migration_requests_action_check
    CHECK (action IN ('up', 'rollback', 'forward', 'resume'));
ALTER TABLE migration_requests DROP CONSTRAINT migration_requests_status_check;
ALTER TABLE migration_requests ADD CONSTRAINT migration_requests_status_check
    CHECK (status IN ('pending_review', 'rejected', 'approved', 'queued',
                      'running', 'success', 'failed', 'partial', 'cancelled'));

ALTER TABLE migration_executions DROP CONSTRAINT migration_executions_action_check;
ALTER TABLE migration_executions ADD CONSTRAINT migration_executions_action_check
    CHECK (action IN ('up', 'rollback', 'forward', 'resume'));
ALTER TABLE migration_executions DROP CONSTRAINT migration_executions_status_check;
ALTER TABLE migration_executions ADD CONSTRAINT migration_executions_status_check
    CHECK (status IN ('running', 'success', 'failed', 'partial'));

ALTER TABLE migration_executions
    -- Kunci rangkaian checkpoint: request up/forward yang memulai rangkaian.
    -- Resume dan rollback batch memakai kunci eksekusi yang menjadi target.
    ADD COLUMN batch_key         BIGINT,
    ADD COLUMN batch_size        INT,
    ADD COLUMN committed_batches INT    NOT NULL DEFAULT 0,
    ADD COLUMN committed_rows    BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN total_batches     INT,
    ADD COLUMN cursor_last       TEXT;

CREATE TABLE migration_batch_checkpoints (
    id            BIGSERIAL   PRIMARY KEY,
    execution_id  BIGINT      NOT NULL REFERENCES migration_executions (id) ON DELETE CASCADE,
    batch_key     BIGINT      NOT NULL,
    direction     TEXT        NOT NULL CHECK (direction IN ('up', 'down')),
    batch_number  INT         NOT NULL CHECK (batch_number > 0),
    cursor_start  TEXT        NOT NULL,
    cursor_end    TEXT        NOT NULL,
    rows_affected BIGINT      NOT NULL DEFAULT 0,
    committed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (batch_key, direction, batch_number)
);

CREATE INDEX migration_batch_checkpoints_execution_idx
    ON migration_batch_checkpoints (execution_id, batch_number);
