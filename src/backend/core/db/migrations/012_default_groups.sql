-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add is_system flag to groups table
ALTER TABLE groups ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0;

-- Seed default system groups.
-- ON CONFLICT DO NOTHING preserves SQLite's INSERT OR IGNORE
-- semantics on Postgres: re-running the migration is safe.
INSERT INTO groups (id, name, description, is_system, created_at, updated_at)
VALUES
    ('grp_admins',   'Admins',    'Full administrative access',          1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('grp_operators','Operators', 'Manage containers and environments',  1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('grp_editors',  'Editors',   'Create and modify resources',         1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('grp_viewers',  'Viewers',   'Read-only access to all resources',   1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (id) DO NOTHING;

-- Assign matching system roles to each group (global scope)
INSERT INTO group_roles (id, group_id, role_id, environment_id, created_at, updated_at)
VALUES
    ('gr_adm_role',  'grp_admins',    'role_admin',    NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('gr_opr_role',  'grp_operators', 'role_operator', NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('gr_edt_role',  'grp_editors',   'role_editor',   NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('gr_vwr_role',  'grp_viewers',   'role_viewer',   NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (id) DO NOTHING;

-- Add the first user (admin) to the Admins group
INSERT INTO group_members (id, group_id, user_id, created_at, updated_at)
SELECT 'gm_first_admin', 'grp_admins', id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM users ORDER BY created_at ASC LIMIT 1
ON CONFLICT (id) DO NOTHING;
