-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE container_backup_runs ADD COLUMN progress_stage TEXT NOT NULL DEFAULT '';
ALTER TABLE container_backup_runs ADD COLUMN progress_message TEXT NOT NULL DEFAULT '';
ALTER TABLE container_backup_runs ADD COLUMN progress_updated_at TEXT;

CREATE INDEX IF NOT EXISTS idx_container_backup_runs_progress ON container_backup_runs(status, progress_updated_at);
