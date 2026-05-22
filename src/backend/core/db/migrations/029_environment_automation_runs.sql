-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE environments ADD COLUMN last_automatic_image_prune_run_at TEXT NOT NULL DEFAULT '';
