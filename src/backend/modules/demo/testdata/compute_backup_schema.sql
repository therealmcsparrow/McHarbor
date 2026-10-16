-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- The compute plugins' backup and snapshot tables, copied verbatim from a demo
-- database so the seed's inserts meet the real constraints. The host schema does
-- not carry them: a plugin adds its tables at Init, so a test on the host schema
-- alone skips the KVM and LXC halves of the seed entirely -- which is how a NULL
-- into plugin_addon_kvm_backup_run_destinations.error reached a running instance.

-- Minimal stand-ins for the rows the seed only reads.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_vms (
    id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS plugin_addon_lxc_containers (
    id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_snapshots (
    id TEXT PRIMARY KEY, vm_id TEXT NOT NULL);

CREATE TABLE plugin_addon_kvm_backup_plans (
    id              TEXT PRIMARY KEY,
    vm_id           TEXT NOT NULL,
    name            TEXT NOT NULL,
    cron            TEXT,
    enabled         INTEGER NOT NULL DEFAULT 0,
    retention_count INTEGER NOT NULL DEFAULT 0,
    retention_days  INTEGER NOT NULL DEFAULT 0,
    full_every      INTEGER NOT NULL DEFAULT 7,
    quiesce         INTEGER NOT NULL DEFAULT 1,
    last_run_at     TEXT,
    next_run_at     TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (vm_id) REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE
);

CREATE TABLE plugin_addon_kvm_backup_plan_destinations (
    plan_id             TEXT NOT NULL REFERENCES plugin_addon_kvm_backup_plans(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    PRIMARY KEY (plan_id, storage_location_id)
);

CREATE TABLE plugin_addon_kvm_backup_runs (
    id                  TEXT PRIMARY KEY,
    plan_id             TEXT REFERENCES plugin_addon_kvm_backup_plans(id) ON DELETE SET NULL,
    vm_id               TEXT NOT NULL,
    environment_id      TEXT NOT NULL,
    operation           TEXT NOT NULL DEFAULT 'backup'
                        CHECK(operation IN ('backup','restore')),
    source_run_id       TEXT REFERENCES plugin_addon_kvm_backup_runs(id) ON DELETE SET NULL,
    status              TEXT NOT NULL
                        CHECK(status IN ('running','success','failure','cancelled')),
    kind                TEXT NOT NULL DEFAULT 'full' CHECK(kind IN ('full','incremental')),
    chain_id            TEXT,
    -- Recorded per run rather than derived: a downgrade is the operator's
    -- explanation for why tonight's "incremental" moved 200 GB.
    fell_back_to_full   INTEGER NOT NULL DEFAULT 0,
    fallback_reason     TEXT,
    consistency         TEXT NOT NULL DEFAULT 'crash',
    bytes_total         INTEGER NOT NULL DEFAULT 0,
    -- The plan can be edited or deleted after a run; the run has to carry its
    -- own truth about what it covered or the restore dialog lies.
    included_disks      TEXT NOT NULL DEFAULT '',
    error               TEXT,
    progress_stage      TEXT NOT NULL DEFAULT '',
    progress_message    TEXT NOT NULL DEFAULT '',
    progress_updated_at TEXT,
    started_at          TEXT NOT NULL,
    completed_at        TEXT,
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    FOREIGN KEY (vm_id) REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE
);

CREATE TABLE plugin_addon_kvm_backup_run_destinations (
    id                    TEXT PRIMARY KEY,
    run_id                TEXT NOT NULL REFERENCES plugin_addon_kvm_backup_runs(id) ON DELETE CASCADE,
    storage_location_id   TEXT NOT NULL,
    storage_location_name TEXT NOT NULL DEFAULT '',
    disk_id               TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL CHECK(status IN ('uploading','success','failure')),
    remote_path           TEXT NOT NULL DEFAULT '',
    key_id                TEXT NOT NULL DEFAULT '',
    bytes_uploaded        INTEGER NOT NULL DEFAULT 0,
    error                 TEXT NOT NULL DEFAULT '',
    uploaded_at           TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE TABLE plugin_addon_kvm_snapshot_schedules (
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
, pause_guest INTEGER NOT NULL DEFAULT 1);

CREATE TABLE plugin_addon_lxc_backup_plans (
    id              TEXT PRIMARY KEY,
    container_id    TEXT NOT NULL,
    name            TEXT NOT NULL,
    cron            TEXT,
    enabled         INTEGER NOT NULL DEFAULT 0,
    retention_count INTEGER NOT NULL DEFAULT 0,
    retention_days  INTEGER NOT NULL DEFAULT 0,
    full_every      INTEGER NOT NULL DEFAULT 7,
    last_run_at     TEXT,
    next_run_at     TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (container_id) REFERENCES plugin_addon_lxc_containers(id) ON DELETE CASCADE
);

CREATE TABLE plugin_addon_lxc_backup_plan_destinations (
    plan_id             TEXT NOT NULL REFERENCES plugin_addon_lxc_backup_plans(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    PRIMARY KEY (plan_id, storage_location_id)
);

CREATE TABLE plugin_addon_lxc_backup_runs (
    id                  TEXT PRIMARY KEY,
    plan_id             TEXT REFERENCES plugin_addon_lxc_backup_plans(id) ON DELETE SET NULL,
    container_id        TEXT NOT NULL,
    container_name      TEXT NOT NULL DEFAULT '',
    environment_id      TEXT NOT NULL,
    operation           TEXT NOT NULL DEFAULT 'backup'
                        CHECK(operation IN ('backup','restore')),
    source_run_id       TEXT,
    status              TEXT NOT NULL
                        CHECK(status IN ('running','success','failure','cancelled')),
    kind                TEXT NOT NULL DEFAULT 'full' CHECK(kind IN ('full','incremental')),
    chain_id            TEXT,
    fell_back_to_full   INTEGER NOT NULL DEFAULT 0,
    fallback_reason     TEXT,
    consistency         TEXT NOT NULL DEFAULT 'filesystem',
    bytes_total         INTEGER NOT NULL DEFAULT 0,
    target_container_id TEXT NOT NULL DEFAULT '',
    error               TEXT,
    progress_stage      TEXT NOT NULL DEFAULT '',
    progress_message    TEXT NOT NULL DEFAULT '',
    progress_updated_at TEXT,
    started_at          TEXT NOT NULL,
    completed_at        TEXT,
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

