-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_guest_health (
    id                   TEXT PRIMARY KEY,
    vm_id                TEXT NOT NULL UNIQUE,
    environment_id       TEXT NOT NULL,
    run_started_at       TEXT,
    checked_at           TEXT NOT NULL,
    reachable            INTEGER NOT NULL DEFAULT 0,
    agent_state          TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    failing_since        TEXT,
    last_ok_at           TEXT,
    last_error           TEXT,
    filesystems          TEXT,
    load1                REAL,
    load5                REAL,
    load15               REAL,
    clock_skew_ms        INTEGER,
    skew_over_count      INTEGER NOT NULL DEFAULT 0,
    skew_alert           INTEGER NOT NULL DEFAULT 0,
    disk_alert           INTEGER NOT NULL DEFAULT 0,
    unsupported          TEXT,
    agent_too_old        INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_guest_health_env
    ON plugin_addon_kvm_guest_health(environment_id);
