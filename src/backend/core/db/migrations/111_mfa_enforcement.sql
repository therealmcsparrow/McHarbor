-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

INSERT INTO settings (id, key, value, category, created_at, updated_at)
SELECT 'set_security_mfa_required', 'security.mfa_required', 'off', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'security.mfa_required');
