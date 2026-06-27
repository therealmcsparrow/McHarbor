-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Allow the 'cancelled' status so CancelRun can transition in-flight runs
-- to a terminal state. The original 043 constraint only allowed
-- ('running','success','failure'), which caused CancelRun to fail with
-- CHECK constraint failed when a user tried to abort a paused/slow backup.
--
-- SQLite cannot DROP a CHECK constraint, so the standard recipe is:
--   1. rename the live table to a temporary name
--   2. recreate the table with the new constraint, preserving all columns
--   3. copy rows from the temp table into the new table
--   4. drop the temp table
--   5. recreate indexes/triggers that referenced the original table

PRAGMA foreign_keys = OFF;

ALTER TABLE container_backup_runs RENAME TO container_backup_runs_old;

CREATE TABLE container_backup_runs (
    id                   TEXT PRIMARY KEY,
    plan_id              TEXT REFERENCES container_backup_plans(id) ON DELETE SET NULL,
    environment_id       TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    container_id         TEXT NOT NULL,
    status               TEXT NOT NULL CHECK(status IN ('running','success','failure','cancelled')),
    archive_path         TEXT,
    archive_size         INTEGER NOT NULL DEFAULT 0,
    error                TEXT,
    started_at           TEXT NOT NULL,
    completed_at         TEXT,
    duration_ms          INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    archive_encryption   TEXT NOT NULL DEFAULT '',
    archive_key_id       TEXT NOT NULL DEFAULT '',
    progress_stage       TEXT NOT NULL DEFAULT '',
    progress_message     TEXT NOT NULL DEFAULT '',
    progress_updated_at  TEXT,
    operation            TEXT NOT NULL DEFAULT 'backup' CHECK(operation IN ('backup','restore')),
    source_run_id        TEXT REFERENCES container_backup_runs(id) ON DELETE SET NULL
);

INSERT INTO container_backup_runs (
    id, plan_id, environment_id, container_id, status,
    archive_path, archive_size, error, started_at, completed_at,
    duration_ms, created_at, updated_at, archive_encryption,
    archive_key_id, progress_stage, progress_message,
    progress_updated_at, operation, source_run_id
)
SELECT
    id, plan_id, environment_id, container_id, status,
    archive_path, archive_size, error, started_at, completed_at,
    duration_ms, created_at, updated_at, archive_encryption,
    archive_key_id, progress_stage, progress_message,
    progress_updated_at, operation, source_run_id
FROM container_backup_runs_old;

DROP TABLE container_backup_runs_old;

CREATE INDEX IF NOT EXISTS idx_container_backup_runs_plan       ON container_backup_runs(plan_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_container  ON container_backup_runs(environment_id, container_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_started    ON container_backup_runs(started_at);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_progress   ON container_backup_runs(status, progress_updated_at);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_operation  ON container_backup_runs(operation, status);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_source     ON container_backup_runs(source_run_id);

PRAGMA foreign_keys = ON;