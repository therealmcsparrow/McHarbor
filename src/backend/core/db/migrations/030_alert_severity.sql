-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE alerts ADD COLUMN severity TEXT NOT NULL DEFAULT 'warning';
