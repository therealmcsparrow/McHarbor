-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- A chronological log of every backup lifecycle event. The Logging
-- tab in the Backup page reads from this table. Writes come from
-- three places: the progress collector (when the run's stage
-- changes), the plan lifecycle hooks (create/update/delete), and
-- the run / destination / restore hooks (status changes).
--
-- One row per event. Severity follows the same convention as the
-- lifecycle_events table: 'info' for neutral, 'success' for
-- completion, 'warning' for retryable warnings, 'error' for
-- permanent failures.
CREATE TABLE IF NOT EXISTS container_backup_log (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE SET NULL,
    plan_id         TEXT REFERENCES container_backup_plans(id) ON DELETE SET NULL,
    plan_name       TEXT,
    run_id          TEXT REFERENCES container_backup_runs(id) ON DELETE SET NULL,
    container_id    TEXT,
    container_name  TEXT,
    action          TEXT NOT NULL,
    phase           TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL CHECK(severity IN ('info','success','warning','error')),
    message         TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT 'backup' CHECK(source IN ('backup','audit')),
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_container_backup_log_env_time
    ON container_backup_log(environment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_container_backup_log_plan_time
    ON container_backup_log(plan_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_container_backup_log_run_time
    ON container_backup_log(run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_container_backup_log_severity_time
    ON container_backup_log(severity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_container_backup_log_action_time
    ON container_backup_log(action, created_at DESC);
