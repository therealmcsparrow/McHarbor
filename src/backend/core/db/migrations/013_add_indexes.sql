-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add missing indexes for frequently queried columns.

CREATE INDEX IF NOT EXISTS idx_stacks_name ON stacks(name);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_appstore_catalog_slug ON appstore_catalog(slug);
CREATE INDEX IF NOT EXISTS idx_environments_is_active ON environments(is_active);
CREATE INDEX IF NOT EXISTS idx_environments_connection_type ON environments(connection_type);
CREATE INDEX IF NOT EXISTS idx_roles_name ON roles(name);
CREATE INDEX IF NOT EXISTS idx_groups_name ON groups(name);
CREATE INDEX IF NOT EXISTS idx_group_members_group_id ON group_members(group_id);
