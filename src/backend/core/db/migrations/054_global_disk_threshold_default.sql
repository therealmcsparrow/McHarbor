-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Global default for Docker disk-usage notification threshold (used when an environment uses the global default).

INSERT INTO settings (id, key, value, category, updated_at)
SELECT 'default-disk-usage-threshold', 'disk_usage_default_threshold_percent', '80', 'disk_usage', datetime('now')
WHERE NOT EXISTS (
    SELECT 1 FROM settings WHERE key = 'disk_usage_default_threshold_percent'
);
