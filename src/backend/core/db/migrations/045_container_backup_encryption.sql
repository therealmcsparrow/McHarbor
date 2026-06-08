-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE container_backup_runs ADD COLUMN archive_encryption TEXT NOT NULL DEFAULT '';
ALTER TABLE container_backup_runs ADD COLUMN archive_key_id TEXT NOT NULL DEFAULT '';
