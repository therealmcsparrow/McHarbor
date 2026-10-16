-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_plans (
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

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_plans_ct
    ON plugin_addon_lxc_backup_plans(container_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_plans_enabled
    ON plugin_addon_lxc_backup_plans(enabled);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_plan_destinations (
    plan_id             TEXT NOT NULL REFERENCES plugin_addon_lxc_backup_plans(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    PRIMARY KEY (plan_id, storage_location_id)
);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_runs (
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

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_ct
    ON plugin_addon_lxc_backup_runs(container_id, started_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_plan
    ON plugin_addon_lxc_backup_runs(plan_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_chain
    ON plugin_addon_lxc_backup_runs(chain_id, started_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_status
    ON plugin_addon_lxc_backup_runs(status, progress_updated_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_env
    ON plugin_addon_lxc_backup_runs(environment_id, started_at);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_run_destinations (
    id                    TEXT PRIMARY KEY,
    run_id                TEXT NOT NULL REFERENCES plugin_addon_lxc_backup_runs(id) ON DELETE CASCADE,
    storage_location_id   TEXT NOT NULL,
    storage_location_name TEXT NOT NULL DEFAULT '',
    part                  TEXT NOT NULL DEFAULT 'rootfs',
    status                TEXT NOT NULL CHECK(status IN ('uploading','success','failure')),
    remote_path           TEXT NOT NULL DEFAULT '',
    key_id                TEXT NOT NULL DEFAULT '',
    bytes_uploaded        INTEGER NOT NULL DEFAULT 0,
    error                 TEXT NOT NULL DEFAULT '',
    uploaded_at           TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_run_dests_run
    ON plugin_addon_lxc_backup_run_destinations(run_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_run_dests_status
    ON plugin_addon_lxc_backup_run_destinations(status);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_chains (
    id           TEXT PRIMARY KEY,
    container_id TEXT NOT NULL,
    plan_id      TEXT,
    listing_name TEXT NOT NULL,
    last_run_id  TEXT,
    last_full_at TEXT,
    last_run_at  TEXT,
    invalidated  INTEGER NOT NULL DEFAULT 0,
    invalid_why  TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_addon_lxc_chains_ct
    ON plugin_addon_lxc_backup_chains(container_id);
