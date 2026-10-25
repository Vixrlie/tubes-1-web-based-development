-- Rotasi paksa pada request berikutnya ketika privilege user naik.
ALTER TABLE sessions ADD COLUMN force_rotate BOOLEAN NOT NULL DEFAULT FALSE;
