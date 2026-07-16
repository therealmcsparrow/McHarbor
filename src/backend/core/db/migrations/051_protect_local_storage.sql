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
VALUES (
    'default-local-backup',
    'Local storage',
    'local',
    1,
    '/mnt/backup',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (id) DO NOTHING;

UPDATE storage_locations
SET
    name = CASE WHEN id = 'default-local-backup' THEN 'Local storage' ELSE name END,
    location_type = CASE WHEN id = 'default-local-backup' THEN 'local' ELSE location_type END,
    enabled = 1,
    base_path = CASE
        WHEN id = 'default-local-backup' AND COALESCE(base_path, '') = '' THEN '/mnt/backup'
        ELSE base_path
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE location_type = 'local'
   OR id = 'default-local-backup';
