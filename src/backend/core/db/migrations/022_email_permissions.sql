-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- ─── Add email_servers permissions to system roles ─────────────────────

-- Operator: gets view + manage
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'email_servers.view',
    '$[#]', 'email_servers.manage'
)
WHERE name = 'Operator' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%email_servers%';

-- Editor: gets view + manage
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'email_servers.view',
    '$[#]', 'email_servers.manage'
)
WHERE name = 'Editor' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%email_servers%';

-- Viewer: gets view only
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'email_servers.view'
)
WHERE name = 'Viewer' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%email_servers%';
