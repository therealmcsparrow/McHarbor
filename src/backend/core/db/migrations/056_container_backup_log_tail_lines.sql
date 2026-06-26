-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Bounded log backup size: cap ContainerLogs at the last N lines instead
-- of "all". Default 10000 lines; 0 = unlimited (legacy behavior).
ALTER TABLE container_backup_plans
    ADD COLUMN log_tail_lines INTEGER NOT NULL DEFAULT 10000;