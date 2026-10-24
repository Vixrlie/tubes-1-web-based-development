-- Versi migration memakai format timestamp YYYYMMDDHHMM yang membutuhkan 12 digit,
-- sehingga kolom version harus BIGINT, bukan INTEGER.

ALTER TABLE migration_history ALTER COLUMN version TYPE BIGINT;
