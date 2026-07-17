-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add `system.manage` to the Admin system role's explicit permission
-- list so the new self-management endpoints (restart, self-update)
-- are visible in the Roles UI. The Admin role already holds the
-- wildcard "*" so this is purely cosmetic — the wildcard covers
-- every action — but the explicit listing keeps the role inspector
-- honest: operators can see at a glance that the Admin role does
-- include the new self-management permission.
--
-- No role besides Admin gets `system.manage`. The wildcard is the
-- only intended source; custom roles that need self-management can
-- be granted the permission explicitly through the role editor.
UPDATE roles
SET permissions = json_insert(permissions, '$[#]', 'system.manage'),
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'Admin'
  AND is_system = 1
  AND json_extract(permissions, '$') NOT LIKE '%system.manage%';
