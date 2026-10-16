-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Backup snapshot configuration and the plan and run columns that go with
-- it.

-- Free-form config on storage locations.
--
-- Plugin-owned storage LocationTypes (e.g. onedrive_personal,
-- onedrive_business, sharepoint) carry their full config in a
-- free-form JSON map. The host's typed columns (client_id,
-- client_secret, tenant_id, ...) cover the common OAuth cases but
-- can't anticipate every plugin's fields — `tenant` (consumers
-- endpoint), `upn`, `site_id`, etc. all need a place to live.
--
-- The map is stored as JSON text. Sensitive values (anything typed
-- 'secret' in the plugin's ConfigSchema) are encrypted at write
-- time the same way the typed columns are. The dispatcher merges
-- the map contents into the cfg passed to the plugin's Test and
-- Upload/Download handlers — typed columns take precedence so the
-- legacy host fields continue to work for plugins that only use
-- the standard keys.

ALTER TABLE storage_locations ADD COLUMN config TEXT NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_container_backup_plans_storage_location ON container_backup_plans(storage_location_id);


-- Snapshot plugin family: per-environment snapshot configs the
-- backup pipeline consults when a plan has consistency_mode='snapshot'.
-- Each row binds an environment to a plugin-registered driver
-- (zfs | btrfs | lvm | synology | truenas | ...) plus the
-- plugin-specific config JSON.
--
-- The driver column is the dispatch key — it matches the plugin's
-- SnapshotAdapterSpec.Driver. Adding a new snapshot plugin requires
-- no schema change here; the plugin registers its Driver at boot.

CREATE TABLE IF NOT EXISTS container_backup_snapshot_configs (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    driver          TEXT NOT NULL,
    plugin_slug     TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    pool_name       TEXT,
    dataset_prefix  TEXT,
    ssh_target      TEXT,
    use_lxd         INTEGER NOT NULL DEFAULT 0,
    config          TEXT NOT NULL DEFAULT '{}',
    last_probed_at  TEXT,
    last_probe      TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshot_configs_env    ON container_backup_snapshot_configs(environment_id);
CREATE INDEX IF NOT EXISTS idx_snapshot_configs_driver ON container_backup_snapshot_configs(driver);
CREATE INDEX IF NOT EXISTS idx_snapshot_configs_plugin ON container_backup_snapshot_configs(plugin_slug);

-- Many-to-many join: a plan can list several snapshot configs (the
-- backup pipeline resolves which one to use at run time based on the
-- plan's snapshot_config_id FK). The join table is reserved for
-- future "snapshot the same container against multiple drivers"
-- workflows and is currently empty for new installs.
CREATE TABLE IF NOT EXISTS container_backup_plan_snapshot_configs (
    plan_id             TEXT NOT NULL REFERENCES container_backup_plans(id) ON DELETE CASCADE,
    snapshot_config_id  TEXT NOT NULL REFERENCES container_backup_snapshot_configs(id) ON DELETE CASCADE,
    PRIMARY KEY (plan_id, snapshot_config_id)
);

CREATE INDEX IF NOT EXISTS idx_plan_snapshot_configs_plan  ON container_backup_plan_snapshot_configs(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_snapshot_configs_cfg   ON container_backup_plan_snapshot_configs(snapshot_config_id);

-- Per-plan consistency mode. Default 'pause' preserves legacy
-- behavior so existing plans keep working without intervention.
ALTER TABLE container_backup_plans ADD COLUMN consistency_mode    TEXT NOT NULL DEFAULT 'pause';
ALTER TABLE container_backup_plans ADD COLUMN snapshot_config_id  TEXT REFERENCES container_backup_snapshot_configs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_container_backup_plans_snapshot_cfg
    ON container_backup_plans(snapshot_config_id);

-- Per-run snapshot audit columns. Populated when
-- consistency_mode='snapshot' and the adapter's Create returned a
-- SnapshotRef; cleared when the deferred Remove fires.
ALTER TABLE container_backup_runs ADD COLUMN snapshot_name        TEXT;
ALTER TABLE container_backup_runs ADD COLUMN snapshot_driver      TEXT;
ALTER TABLE container_backup_runs ADD COLUMN snapshot_mount_path  TEXT;
ALTER TABLE container_backup_runs ADD COLUMN snapshot_created_at  TEXT;
ALTER TABLE container_backup_runs ADD COLUMN snapshot_removed_at  TEXT;
ALTER TABLE container_backup_runs ADD COLUMN snapshot_error       TEXT;

CREATE INDEX IF NOT EXISTS idx_container_backup_runs_snapshot_driver
    ON container_backup_runs(snapshot_driver);


-- Persist the container name on the backup run itself.
--
-- Until now a run carried only `container_id`, and every read path
-- recovered the human-readable name with
-- `LEFT JOIN container_backup_plans p ON r.plan_id = p.id`. That join
-- returns NULL for the two most common cases:
--
--   * ad-hoc runs (Backup now / restore), which are created with
--     plan_id NULL, and
--   * runs whose plan was deleted afterwards — the FK is
--     ON DELETE SET NULL, so the name disappears with the plan.
--
-- In both cases the UI fell back to the short container id, so the
-- backup list showed a hex string where a name belonged. The name is
-- known at run-creation time (the ad-hoc path already resolves it to
-- build the archive path), so store it on the row where it cannot be
-- orphaned.

ALTER TABLE container_backup_runs ADD COLUMN container_name TEXT NOT NULL DEFAULT '';

-- Backfill 1: the run's own plan, when it still exists.
UPDATE container_backup_runs
SET container_name = COALESCE((
        SELECT p.container_name
        FROM container_backup_plans p
        WHERE p.id = container_backup_runs.plan_id
          AND p.container_name <> ''
    ), '')
WHERE container_name = '' AND plan_id IS NOT NULL;

-- Backfill 2: any plan pointing at the same container in the same
-- environment. Covers runs whose own plan was deleted and ad-hoc runs
-- for a container that also has a schedule.
UPDATE container_backup_runs
SET container_name = COALESCE((
        SELECT p.container_name
        FROM container_backup_plans p
        WHERE p.environment_id = container_backup_runs.environment_id
          AND p.container_id = container_backup_runs.container_id
          AND p.container_name <> ''
        ORDER BY p.updated_at DESC
        LIMIT 1
    ), '')
WHERE container_name = '';

-- Backfill 3: the backup log, which snapshots the name per event.
UPDATE container_backup_runs
SET container_name = COALESCE((
        SELECT l.container_name
        FROM container_backup_log l
        WHERE l.run_id = container_backup_runs.id
          AND l.container_name IS NOT NULL
          AND l.container_name <> ''
        ORDER BY l.created_at DESC
        LIMIT 1
    ), '')
WHERE container_name = '';

-- Anything still empty belongs to a container that was never named in
-- this database. The read path resolves those against the live
-- orchestrator on the next list call and persists the result.


-- Persist what a run actually captured (or restored) on the run row.
--
-- A run row records where the archive went and how big it was, but not
-- *what is in it*. The Backups overview needs that to label a run as a
-- full or a partial backup/restore, and to list the contents behind an
-- info button.
--
-- The selection cannot be recovered from the plan at read time:
--
--   * ad-hoc runs ("Backup now") are built from an unsaved in-memory
--     plan and land with plan_id NULL, and
--   * a saved plan can be edited or deleted after the run — the FK is
--     ON DELETE SET NULL — so its current flags say nothing about what
--     an older run contains.
--
-- Both columns hold a JSON array of the item keys Service.Options
-- already mints: "config", "image", "filesystem", "logs" and
-- "mount:<destination>". available_items is the set the container (for a
-- backup) or the source archive (for a restore) offered at run time;
-- included_items is the subset actually captured or restored. A run is
-- "full" when included covers all of available, "partial" otherwise.
-- Snapshotting both keeps the label historically honest after the
-- container gains or loses a mount.
--
-- No backfill: migrations here run verbatim on SQLite, Postgres and
-- MySQL, and every JSON aggregate is spelled differently on all three.
-- Rows written before this column keep both columns empty; the read path
-- falls back to the joined plan's flags for included_items and the UI
-- degrades to a plain "Backup" / "Restore" label with no full/partial
-- claim rather than guessing one.

ALTER TABLE container_backup_runs ADD COLUMN included_items TEXT NOT NULL DEFAULT '';
ALTER TABLE container_backup_runs ADD COLUMN available_items TEXT NOT NULL DEFAULT '';


-- Per-option capture rules for a backup plan.
--
-- A plan could only say "include this option or not", via the
-- include_config / include_logs / include_filesystem / include_image
-- booleans and the selected_mounts list. Each option now carries a mode
-- of off / choose / full, and `choose` carries the path prefixes to
-- capture.
--
-- Stored as a JSON object keyed by the same option keys the restore side
-- already uses (config, logs, filesystem, image, mount:<target>), so one
-- vocabulary describes both halves:
--
--   {"config":{"mode":"choose","paths":["Config.Env","HostConfig.PortBindings"]},
--    "filesystem":{"mode":"off"},
--    "mount:/var/www/html":{"mode":"choose","paths":["/config","/themes"]}}
--
-- Empty string means "no per-option rules recorded". Those are read back
-- by deriving the modes from the legacy booleans, so existing plans keep
-- capturing exactly what they captured before. The booleans are still
-- written alongside, so any reader that has not learned about selections
-- stays correct.
ALTER TABLE container_backup_plans ADD COLUMN selections TEXT NOT NULL DEFAULT '';


-- A backup provider's rows in the host's backup log.
--
-- plan_id and run_id carry foreign keys to the container tables, so a virtual
-- machine's ids cannot go in them. These columns carry a provider's own ids
-- instead, and subject_kind says which source the row came from so the feed
-- can label and filter it.
ALTER TABLE container_backup_log ADD COLUMN subject_kind TEXT NOT NULL DEFAULT 'container';
ALTER TABLE container_backup_log ADD COLUMN ref_plan_id TEXT;
ALTER TABLE container_backup_log ADD COLUMN ref_run_id TEXT;

CREATE INDEX IF NOT EXISTS idx_container_backup_log_subject_time
    ON container_backup_log(subject_kind, created_at DESC);
