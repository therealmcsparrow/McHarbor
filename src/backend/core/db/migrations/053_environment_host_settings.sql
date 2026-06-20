-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Per-environment host settings: retention, pruning, auto-update window, and disk threshold override.

ALTER TABLE environments ADD COLUMN log_retention_days INTEGER NOT NULL DEFAULT 7;
ALTER TABLE environments ADD COLUMN container_prune_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN container_prune_stopped_days INTEGER NOT NULL DEFAULT 7;
ALTER TABLE environments ADD COLUMN image_prune_dangling_only INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN auto_update_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN auto_update_window_start TEXT NOT NULL DEFAULT '02:00';
ALTER TABLE environments ADD COLUMN auto_update_window_end TEXT NOT NULL DEFAULT '05:00';
ALTER TABLE environments ADD COLUMN auto_update_days TEXT NOT NULL DEFAULT 'mon,tue,wed,thu,fri,sat,sun';
ALTER TABLE environments ADD COLUMN metric_retention_hours INTEGER NOT NULL DEFAULT 24;
ALTER TABLE environments ADD COLUMN docker_disk_usage_use_global_default INTEGER NOT NULL DEFAULT 1;
