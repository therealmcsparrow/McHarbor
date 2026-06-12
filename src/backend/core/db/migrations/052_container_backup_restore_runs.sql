-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE container_backup_runs ADD COLUMN operation TEXT NOT NULL DEFAULT 'backup' CHECK(operation IN ('backup','restore'));
ALTER TABLE container_backup_runs ADD COLUMN source_run_id TEXT REFERENCES container_backup_runs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_container_backup_runs_operation ON container_backup_runs(operation, status);
CREATE INDEX IF NOT EXISTS idx_container_backup_runs_source ON container_backup_runs(source_run_id);
