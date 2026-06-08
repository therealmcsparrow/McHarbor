-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS container_backup_plans (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    environment_id      TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    container_id        TEXT NOT NULL,
    container_name      TEXT NOT NULL DEFAULT '',
    storage_location_id TEXT REFERENCES storage_locations(id) ON DELETE SET NULL,
    include_config      INTEGER NOT NULL DEFAULT 1,
    include_logs        INTEGER NOT NULL DEFAULT 0,
    include_filesystem  INTEGER NOT NULL DEFAULT 0,
    include_image       INTEGER NOT NULL DEFAULT 0,
    selected_mounts     TEXT NOT NULL DEFAULT '[]',
    cron                TEXT,
    enabled             INTEGER NOT NULL DEFAULT 0,
    last_run_at         TEXT,
    next_run_at         TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_container_backup_plans_container ON container_backup_plans(environment_id, container_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_plans_enabled ON container_backup_plans(enabled);

CREATE TABLE IF NOT EXISTS container_backup_runs (
    id             TEXT PRIMARY KEY,
    plan_id        TEXT REFERENCES container_backup_plans(id) ON DELETE SET NULL,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    container_id   TEXT NOT NULL,
    status         TEXT NOT NULL CHECK(status IN ('running','success','failure')),
    archive_path   TEXT,
    archive_size   INTEGER NOT NULL DEFAULT 0,
    error          TEXT,
    started_at     TEXT NOT NULL,
    completed_at   TEXT,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_container_backup_runs_plan ON container_backup_runs(plan_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_container ON container_backup_runs(environment_id, container_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_started ON container_backup_runs(started_at);
