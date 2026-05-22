-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Performance indexes for frequently queried columns.
-- Only includes indexes not already created in previous migrations.

-- ─── audit_logs ─────────────────────────────────────────────────────────
-- Filtered by action in List(); existing: timestamp, user_id, (entity_type, entity_id)
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_environment_id ON audit_logs(environment_id);

-- ─── container_events ───────────────────────────────────────────────────
-- Queried by container_id and action; existing: environment_id, timestamp
CREATE INDEX IF NOT EXISTS idx_container_events_container_id ON container_events(container_id);
CREATE INDEX IF NOT EXISTS idx_container_events_action ON container_events(action);

-- ─── schedules ──────────────────────────────────────────────────────────
-- FK env_id has no index
CREATE INDEX IF NOT EXISTS idx_schedules_env_id ON schedules(env_id);

-- ─── webhooks ───────────────────────────────────────────────────────────
-- FK environment_id and is_active filter have no indexes
CREATE INDEX IF NOT EXISTS idx_webhooks_environment_id ON webhooks(environment_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_is_active ON webhooks(is_active);

-- ─── settings ───────────────────────────────────────────────────────────
-- Queried by category; key already has UNIQUE constraint
CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);

-- ─── user_roles ─────────────────────────────────────────────────────────
-- FK role_id has no single-column index (only user_id and composite unique)
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role_id);

-- ─── group_roles ────────────────────────────────────────────────────────
-- FK role_id has no single-column index (only group_id and composite unique)
CREATE INDEX IF NOT EXISTS idx_group_roles_role ON group_roles(role_id);

-- ─── appstore_installed ─────────────────────────────────────────────────
-- FK environment_id has no index
CREATE INDEX IF NOT EXISTS idx_appstore_installed_env ON appstore_installed(environment_id);

-- ─── git_repos ──────────────────────────────────────────────────────────
-- FK env_id has no index
CREATE INDEX IF NOT EXISTS idx_git_repos_env_id ON git_repos(env_id);

-- ─── scans ──────────────────────────────────────────────────────────────
-- Filtered by status for pending/running scan lookup
CREATE INDEX IF NOT EXISTS idx_scans_status ON scans(status);
