-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE scans ADD COLUMN environment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE scans ADD COLUMN error_output TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_scans_environment ON scans(environment_id);
CREATE INDEX IF NOT EXISTS idx_scans_image_ref ON scans(image_ref);
CREATE INDEX IF NOT EXISTS idx_scans_status ON scans(status);
