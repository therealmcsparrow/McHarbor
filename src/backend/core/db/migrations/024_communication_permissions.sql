-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- ─── Add communications permissions to system roles ─────────────────────

-- Operator: gets view + manage
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'communications.view',
    '$[#]', 'communications.manage'
)
WHERE name = 'Operator' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%communications%';

-- Editor: gets view + manage
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'communications.view',
    '$[#]', 'communications.manage'
)
WHERE name = 'Editor' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%communications%';

-- Viewer: gets view only
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'communications.view'
)
WHERE name = 'Viewer' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%communications%';
