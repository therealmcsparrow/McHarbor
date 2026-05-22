-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- ─── RBAC: Add is_system flag to existing roles table ─────────────────
ALTER TABLE roles ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0;

-- ─── Seed system roles ────────────────────────────────────────────────
INSERT OR IGNORE INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
VALUES
    ('role_admin', 'Admin', 'Full access to all resources and settings', '["*"]', 1, datetime('now'), datetime('now')),
    ('role_operator', 'Operator', 'Manage containers, images, volumes, networks, stacks, and Kubernetes resources',
     '["containers.view","containers.manage","containers.delete","images.view","images.manage","images.delete","volumes.view","volumes.manage","volumes.delete","networks.view","networks.manage","networks.delete","stacks.view","stacks.manage","stacks.delete","terminal.access","logs.view","settings.view","users.view","pods.view","pods.manage","pods.delete","deployments.view","deployments.manage","deployments.delete","k8s_services.view","k8s_services.manage","k8s_services.delete","namespaces.view","environments.view","groups.view","roles.view"]',
     1, datetime('now'), datetime('now')),
    ('role_editor', 'Editor', 'View and manage resources without delete or admin access',
     '["containers.view","containers.manage","images.view","images.manage","volumes.view","volumes.manage","networks.view","networks.manage","stacks.view","stacks.manage","terminal.access","logs.view","pods.view","pods.manage","deployments.view","deployments.manage","k8s_services.view","k8s_services.manage","namespaces.view","environments.view","groups.view","roles.view"]',
     1, datetime('now'), datetime('now')),
    ('role_viewer', 'Viewer', 'Read-only access to all resources',
     '["containers.view","images.view","volumes.view","networks.view","stacks.view","logs.view","settings.view","users.view","pods.view","deployments.view","k8s_services.view","namespaces.view","environments.view","groups.view","roles.view"]',
     1, datetime('now'), datetime('now'));

-- ─── Groups ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Group Members ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS group_members (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_members_unique ON group_members(group_id, user_id);
CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members(user_id);

-- ─── Group Roles (per-environment) ───────────────────────────────────
CREATE TABLE IF NOT EXISTS group_roles (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_roles_unique ON group_roles(group_id, role_id, environment_id);
CREATE INDEX IF NOT EXISTS idx_group_roles_group ON group_roles(group_id);

-- ─── API Keys ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    scopes TEXT NOT NULL DEFAULT '[]',
    expires_at TEXT,
    last_used_at TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);

-- ─── Unique index on user_roles ───────────────────────────────────────
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_roles_unique ON user_roles(user_id, role_id, environment_id);

-- ─── Auto-assign Admin role to first user ─────────────────────────────
INSERT OR IGNORE INTO user_roles (id, user_id, role_id, environment_id, created_at, updated_at)
SELECT
    'ur_first_admin',
    u.id,
    'role_admin',
    NULL,
    datetime('now'),
    datetime('now')
FROM users u
ORDER BY u.created_at ASC
LIMIT 1;
