-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS instance_backup_schedules (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    cron             TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    key_kind         TEXT NOT NULL,
    key_secret_enc   TEXT NOT NULL,
    key_id           TEXT NOT NULL DEFAULT '',
    include_metrics  INTEGER NOT NULL DEFAULT 0,
    location_id      TEXT,
    retention_count  INTEGER NOT NULL DEFAULT 7,
    retention_days   INTEGER NOT NULL DEFAULT 0,
    verify_after     INTEGER NOT NULL DEFAULT 1,
    last_run_at      TEXT,
    next_run_at      TEXT,
    last_status      TEXT NOT NULL DEFAULT '',
    last_error       TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_instance_backup_schedules_next ON instance_backup_schedules(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_instance_backup_schedules_location ON instance_backup_schedules(location_id);

CREATE TABLE IF NOT EXISTS instance_backup_exports (
    id                  TEXT PRIMARY KEY,
    status              TEXT NOT NULL DEFAULT 'running',
    trigger_kind        TEXT NOT NULL DEFAULT 'manual',
    schedule_id         TEXT,
    created_by          TEXT NOT NULL DEFAULT '',
    file_name           TEXT NOT NULL DEFAULT '',
    size_bytes          INTEGER NOT NULL DEFAULT 0,
    manifest            TEXT NOT NULL DEFAULT '',
    key_kind            TEXT NOT NULL DEFAULT '',
    key_id              TEXT NOT NULL DEFAULT '',
    include_metrics     INTEGER NOT NULL DEFAULT 0,
    note                TEXT NOT NULL DEFAULT '',
    error               TEXT NOT NULL DEFAULT '',
    location_id         TEXT,
    location_key        TEXT NOT NULL DEFAULT '',
    location_status     TEXT NOT NULL DEFAULT '',
    location_error      TEXT NOT NULL DEFAULT '',
    verify_status       TEXT NOT NULL DEFAULT '',
    verify_error        TEXT NOT NULL DEFAULT '',
    verified_at         TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_instance_backup_exports_created ON instance_backup_exports(created_at);
CREATE INDEX IF NOT EXISTS idx_instance_backup_exports_schedule ON instance_backup_exports(schedule_id, created_at);
CREATE INDEX IF NOT EXISTS idx_instance_backup_exports_status ON instance_backup_exports(status);
