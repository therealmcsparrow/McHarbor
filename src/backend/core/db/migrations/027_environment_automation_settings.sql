-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE environments ADD COLUMN scheduled_update_check_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN automatic_image_pruning_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';
