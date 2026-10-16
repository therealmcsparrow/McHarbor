-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE workflows ADD COLUMN self_host_authorized INTEGER NOT NULL DEFAULT 0;
