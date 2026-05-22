-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE user_roles ADD COLUMN stack_name TEXT;
ALTER TABLE group_roles ADD COLUMN stack_name TEXT;

DROP INDEX IF EXISTS idx_user_roles_unique;
DROP INDEX IF EXISTS idx_group_roles_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_roles_unique
ON user_roles(user_id, role_id, COALESCE(environment_id, ''), COALESCE(stack_name, ''));

CREATE UNIQUE INDEX IF NOT EXISTS idx_group_roles_unique
ON group_roles(group_id, role_id, COALESCE(environment_id, ''), COALESCE(stack_name, ''));

CREATE INDEX IF NOT EXISTS idx_user_roles_scope
ON user_roles(user_id, environment_id, stack_name);

CREATE INDEX IF NOT EXISTS idx_group_roles_scope
ON group_roles(group_id, environment_id, stack_name);
