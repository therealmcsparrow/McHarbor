-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Add is_system flag to groups table
ALTER TABLE groups ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0;

-- Seed default system groups
INSERT OR IGNORE INTO groups (id, name, description, is_system, created_at, updated_at)
VALUES
    ('grp_admins',   'Admins',    'Full administrative access',          1, datetime('now'), datetime('now')),
    ('grp_operators','Operators', 'Manage containers and environments',  1, datetime('now'), datetime('now')),
    ('grp_editors',  'Editors',   'Create and modify resources',         1, datetime('now'), datetime('now')),
    ('grp_viewers',  'Viewers',   'Read-only access to all resources',   1, datetime('now'), datetime('now'));

-- Assign matching system roles to each group (global scope)
INSERT OR IGNORE INTO group_roles (id, group_id, role_id, environment_id, created_at, updated_at)
VALUES
    ('gr_adm_role',  'grp_admins',    'role_admin',    NULL, datetime('now'), datetime('now')),
    ('gr_opr_role',  'grp_operators', 'role_operator', NULL, datetime('now'), datetime('now')),
    ('gr_edt_role',  'grp_editors',   'role_editor',   NULL, datetime('now'), datetime('now')),
    ('gr_vwr_role',  'grp_viewers',   'role_viewer',   NULL, datetime('now'), datetime('now'));

-- Add the first user (admin) to the Admins group
INSERT OR IGNORE INTO group_members (id, group_id, user_id, created_at, updated_at)
SELECT 'gm_first_admin', 'grp_admins', id, datetime('now'), datetime('now')
FROM users ORDER BY created_at ASC LIMIT 1;
