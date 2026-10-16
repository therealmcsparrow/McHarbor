-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Definition, schedule and retention in one row, as container_backup_plans
-- does. full_every is the incremental safety valve: however well the chain
-- logic works, a chain that is never restarted is one lost bitmap away from
-- a very long restore.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_backup_plans (
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

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_plans_vm
    ON plugin_addon_kvm_backup_plans(vm_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_plans_enabled
    ON plugin_addon_kvm_backup_plans(enabled);

-- Backups and restores share one table, keyed by operation, exactly as
-- container_backup_runs does. History, progress, cancellation and the polling
-- endpoint are then one implementation rather than two that drift.
--
-- kind records what was actually taken, never what was requested: the agent
-- decides, because only it can see whether the dirty bitmap survived.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_backup_runs (
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

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_runs_vm
    ON plugin_addon_kvm_backup_runs(vm_id, started_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_runs_plan
    ON plugin_addon_kvm_backup_runs(plan_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_runs_chain
    ON plugin_addon_kvm_backup_runs(chain_id, started_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_runs_status
    ON plugin_addon_kvm_backup_runs(status, progress_updated_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_runs_source
    ON plugin_addon_kvm_backup_runs(source_run_id);

-- A plan writes to every destination it names. Many-to-many because the whole
-- point of off-host backup is that one copy is not a backup.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_backup_plan_destinations (
    plan_id             TEXT NOT NULL REFERENCES plugin_addon_kvm_backup_plans(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    PRIMARY KEY (plan_id, storage_location_id)
);

-- One row per destination per run. A destination that failed while its
-- siblings succeeded is a partial success, and the operator needs to see
-- which — not a single status for the whole run.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_backup_run_destinations (
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

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_run_dests_run
    ON plugin_addon_kvm_backup_run_destinations(run_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_backup_run_dests_status
    ON plugin_addon_kvm_backup_run_destinations(status);

-- The chain table already exists from 0005. granularity is recorded so a
-- bitmap that was rebuilt at a different one is treated as a different
-- bitmap, and last_run_id is what makes the ancestor walk cheap at restore
-- and at prune.
ALTER TABLE plugin_addon_kvm_backup_chains ADD COLUMN granularity INTEGER;
ALTER TABLE plugin_addon_kvm_backup_chains ADD COLUMN last_run_id TEXT;
ALTER TABLE plugin_addon_kvm_backup_chains ADD COLUMN plan_id TEXT;
