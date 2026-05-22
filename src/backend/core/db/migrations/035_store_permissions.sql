-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add store permissions to system roles

-- Operator: gets view + manage
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'store_apps.view',
    '$[#]', 'store_apps.manage',
    '$[#]', 'store_nodes.view',
    '$[#]', 'store_nodes.manage',
    '$[#]', 'store_widgets.view',
    '$[#]', 'store_widgets.manage'
)
WHERE name = 'Operator' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%store_apps%';

-- Editor: gets view + manage
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'store_apps.view',
    '$[#]', 'store_apps.manage',
    '$[#]', 'store_nodes.view',
    '$[#]', 'store_nodes.manage',
    '$[#]', 'store_widgets.view',
    '$[#]', 'store_widgets.manage'
)
WHERE name = 'Editor' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%store_apps%';

-- Viewer: gets view only
UPDATE roles
SET permissions = json_insert(
    permissions,
    '$[#]', 'store_apps.view',
    '$[#]', 'store_nodes.view',
    '$[#]', 'store_widgets.view'
)
WHERE name = 'Viewer' AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%store_apps%';
