-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE license_notices ADD COLUMN user_id TEXT;

CREATE INDEX IF NOT EXISTS idx_license_notices_user_kind ON license_notices(user_id, kind, shown_at);
