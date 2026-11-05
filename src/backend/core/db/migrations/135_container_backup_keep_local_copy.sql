-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE container_backup_plans ADD COLUMN keep_local_copy INTEGER NOT NULL DEFAULT 1;
ALTER TABLE container_backup_runs ADD COLUMN local_copy_dropped INTEGER NOT NULL DEFAULT 0;
