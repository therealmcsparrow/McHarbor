-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

UPDATE role_presets
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(role_presets.permissions)
           UNION
           SELECT value FROM json_each(json_array('runbooks.view'))
       )
   )
 WHERE key IN ('container_operator', 'operator', 'developer', 'platform_admin', 'viewer');

UPDATE role_presets
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(role_presets.permissions)
           UNION
           SELECT value FROM json_each(json_array('runbooks.run'))
       )
   )
 WHERE key IN ('container_operator', 'operator', 'developer', 'platform_admin');

UPDATE role_presets
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(role_presets.permissions)
           UNION
           SELECT value FROM json_each(json_array('runbooks.manage'))
       )
   )
 WHERE key IN ('operator', 'platform_admin');
