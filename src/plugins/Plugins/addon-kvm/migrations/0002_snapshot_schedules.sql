-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_snapshot_schedules (
    id    TEXT PRIMARY KEY,
    vm_id TEXT NOT NULL REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    cron    TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,

    keep_count INTEGER NOT NULL DEFAULT 7,

    with_memory INTEGER NOT NULL DEFAULT 0,

    last_run_at  TEXT,
    next_run_at  TEXT,
    last_error   TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_schedules_vm
    ON plugin_addon_kvm_snapshot_schedules(vm_id);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_schedules_enabled
    ON plugin_addon_kvm_snapshot_schedules(enabled);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_snapshot_runs (
    id          TEXT PRIMARY KEY,
    schedule_id TEXT REFERENCES plugin_addon_kvm_snapshot_schedules(id) ON DELETE CASCADE,
    vm_id       TEXT NOT NULL,
    status      TEXT NOT NULL CHECK(status IN ('running','success','failure')),
    snapshot_id   TEXT,
    snapshot_name TEXT,
    pruned       INTEGER NOT NULL DEFAULT 0,
    error        TEXT,
    started_at   TEXT NOT NULL,
    completed_at TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_runs_schedule
    ON plugin_addon_kvm_snapshot_runs(schedule_id);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_runs_started
    ON plugin_addon_kvm_snapshot_runs(started_at);

ALTER TABLE plugin_addon_kvm_snapshots
    ADD COLUMN schedule_id TEXT;

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshots_schedule
    ON plugin_addon_kvm_snapshots(schedule_id);
