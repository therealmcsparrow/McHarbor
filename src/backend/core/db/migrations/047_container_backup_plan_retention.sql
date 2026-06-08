-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE container_backup_plans ADD COLUMN retention_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE container_backup_plans ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 0;
