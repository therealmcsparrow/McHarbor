-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

INSERT INTO storage_locations (
    id,
    name,
    location_type,
    enabled,
    base_path,
    created_at,
    updated_at
)
SELECT
    'default-local-backup',
    'Local backup storage',
    'local',
    1,
    '/mnt/backup',
    datetime('now'),
    datetime('now')
WHERE NOT EXISTS (
    SELECT 1
    FROM storage_locations
    WHERE location_type = 'local'
      AND base_path = '/mnt/backup'
);
