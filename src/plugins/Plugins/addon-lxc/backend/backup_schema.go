// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// backupSchemaStatements is 0003_ct_backups.sql, statement by statement.
//
// Duplicated rather than read from the file because a plugin cannot reach its own
// migration directory at runtime — the installed tree holds the tarball's copy and
// nothing maps it into the process. Both are IF NOT EXISTS, so whichever runs
// first wins and the second is a no-op.
//
// # The dialect
//
// Written as SQLite, which is what the host's statement translator reads: it
// rewrites each one for Postgres, MySQL/MariaDB and SQL Server on an installation
// that points MCHARBOR_DB_DRIVER elsewhere. Nothing here reaches for an idiom the
// translator does not cover — no julianday, no INSERT OR IGNORE, no partial index
// — so all four backends get the same shape.
//
// # The four decisions worth knowing before editing these
//
// There is no quiesce column on the plans table. A machine backup can legitimately
// be crash-consistent because a guest filesystem journal replays; a copy of a
// rootfs taken while a package manager is halfway through it cannot be told from a
// good one, so the agent refuses that case outright and there is nothing for an
// operator to switch off.
//
// container_id on the runs table carries no foreign key, and that is the one place
// this schema departs from addon-kvm's. A backup whose container has been destroyed
// is exactly the backup somebody goes looking for, and restoring it into a fresh
// container is a first-class mode — so the history has to outlive its subject
// rather than cascading away with it.
//
// There is one chain row per container rather than per volume, which is the whole
// difference from the machine chains: a container has one rootfs. listing_name is
// GNU tar's incremental listing on the host, the direct analogue of a dirty bitmap,
// and it is lost the same ways — which is why invalidation is recorded rather than
// inferred.
//
// The run_destinations rows are keyed by part as well as by destination: 'manifest'
// and 'rootfs' are two objects rather than one, because the manifest is kilobytes
// and the rootfs is tens of gigabytes, and wrapping them together would mean
// staging a second copy of the large one to prepend the small one.
var backupSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_plans (
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
)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_plans_ct
    ON plugin_addon_lxc_backup_plans(container_id)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_plans_enabled
    ON plugin_addon_lxc_backup_plans(enabled)`,
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_plan_destinations (
    plan_id             TEXT NOT NULL REFERENCES plugin_addon_lxc_backup_plans(id) ON DELETE CASCADE,
    storage_location_id TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    PRIMARY KEY (plan_id, storage_location_id)
)`,
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_runs (
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
)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_ct
    ON plugin_addon_lxc_backup_runs(container_id, started_at)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_plan
    ON plugin_addon_lxc_backup_runs(plan_id)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_chain
    ON plugin_addon_lxc_backup_runs(chain_id, started_at)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_status
    ON plugin_addon_lxc_backup_runs(status, progress_updated_at)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_runs_env
    ON plugin_addon_lxc_backup_runs(environment_id, started_at)`,
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_run_destinations (
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
)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_run_dests_run
    ON plugin_addon_lxc_backup_run_destinations(run_id)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_backup_run_dests_status
    ON plugin_addon_lxc_backup_run_destinations(status)`,
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_chains (
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
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_addon_lxc_chains_ct
    ON plugin_addon_lxc_backup_chains(container_id)`,
	// 0004_backup_idmaps.sql. A side table rather than columns on the runs table
	// because CREATE TABLE IF NOT EXISTS is idempotent in both the migration and
	// here, where ADD COLUMN would fail whichever of the two ran second. id is the
	// run id; a legacy run has no row, and its range is inferred at restore.
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_run_idmaps (
    id         TEXT PRIMARY KEY REFERENCES plugin_addon_lxc_backup_runs(id) ON DELETE CASCADE,
    privileged INTEGER NOT NULL DEFAULT 0,
    uid_start  INTEGER NOT NULL DEFAULT 0,
    uid_count  INTEGER NOT NULL DEFAULT 0,
    gid_start  INTEGER NOT NULL DEFAULT 0,
    gid_count  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)`,
	// 0005_backup_idshifts.sql, a side table for the same reason as 0004. id is
	// the restore run's id; a run without a row predates the field, or was never
	// answered by the agent.
	`CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_run_idshifts (
    id         TEXT PRIMARY KEY REFERENCES plugin_addon_lxc_backup_runs(id) ON DELETE CASCADE,
    id_shift   TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)`,
}
