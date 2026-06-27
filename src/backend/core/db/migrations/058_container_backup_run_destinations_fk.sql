-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Migration 057 rebuilt `container_backup_runs` with the new
-- CHECK(status IN ('running','success','failure','cancelled')) constraint
-- by renaming the live table to `container_backup_runs_old`, recreating
-- `container_backup_runs`, copying data, and dropping the temp table.
--
-- SQLite propagates the rename to foreign keys in other tables even when
-- `PRAGMA foreign_keys = OFF` is set: the FK in
-- `container_backup_run_destinations.run_id` was rewritten from
-- `container_backup_runs(id)` to `container_backup_runs_old(id)` as soon as
-- the RENAME ran. After the DROP, that table no longer existed, so any
-- INSERT into `container_backup_run_destinations` failed with
-- "no such table: main.container_backup_runs_old".
--
-- Rebuild `container_backup_run_destinations` the same way: rename, recreate
-- with the correct FK against `container_backup_runs(id)`, copy data, drop.

ALTER TABLE container_backup_run_destinations RENAME TO container_backup_run_destinations_old;

CREATE TABLE container_backup_run_destinations (
    id                    TEXT PRIMARY KEY,
    run_id                TEXT NOT NULL REFERENCES container_backup_runs(id) ON DELETE CASCADE,
    storage_location_id   TEXT REFERENCES storage_locations(id) ON DELETE SET NULL,
    storage_location_name TEXT NOT NULL DEFAULT '',
    location_type         TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL CHECK(status IN ('uploading','success','failure')),
    remote_path           TEXT NOT NULL DEFAULT '',
    error                 TEXT NOT NULL DEFAULT '',
    uploaded_at           TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    bytes_uploaded        INTEGER NOT NULL DEFAULT 0,
    bytes_total           INTEGER NOT NULL DEFAULT 0
);

INSERT INTO container_backup_run_destinations (
    id, run_id, storage_location_id, storage_location_name, location_type,
    status, remote_path, error, uploaded_at, created_at, updated_at,
    bytes_uploaded, bytes_total
)
SELECT
    id, run_id, storage_location_id, storage_location_name, location_type,
    status, remote_path, error, uploaded_at, created_at, updated_at,
    bytes_uploaded, bytes_total
FROM container_backup_run_destinations_old;

DROP TABLE container_backup_run_destinations_old;

CREATE INDEX IF NOT EXISTS idx_container_backup_run_destinations_run          ON container_backup_run_destinations(run_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_run_destinations_location    ON container_backup_run_destinations(storage_location_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_run_destinations_status      ON container_backup_run_destinations(status);