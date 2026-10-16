-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE environments ADD COLUMN host_update_remove_unused INTEGER NOT NULL DEFAULT 0;

ALTER TABLE host_update_self ADD COLUMN host_update_remove_unused INTEGER NOT NULL DEFAULT 0;
