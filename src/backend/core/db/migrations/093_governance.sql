-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS approval_rules (
    id                  TEXT PRIMARY KEY,
    category            TEXT NOT NULL,
    environment_id      TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    required_approvers  INTEGER NOT NULL DEFAULT 1,
    approver_permission TEXT NOT NULL DEFAULT 'approvals.decide',
    expiry_minutes      INTEGER NOT NULL DEFAULT 1440,
    description         TEXT NOT NULL DEFAULT '',
    created_by          TEXT,
    created_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_approval_rules_category ON approval_rules(category, enabled);
CREATE INDEX IF NOT EXISTS idx_approval_rules_environment ON approval_rules(environment_id);

CREATE TABLE IF NOT EXISTS approval_requests (
    id                  TEXT PRIMARY KEY,
    rule_id             TEXT,
    category            TEXT NOT NULL,
    requester_id        TEXT NOT NULL,
    requester_name      TEXT NOT NULL DEFAULT '',
    environment_id      TEXT,
    method              TEXT NOT NULL,
    path                TEXT NOT NULL,
    route_pattern       TEXT NOT NULL DEFAULT '',
    resource_name       TEXT NOT NULL DEFAULT '',
    body_redacted       TEXT NOT NULL DEFAULT '',
    body_encrypted      TEXT NOT NULL DEFAULT '',
    api_key_id          TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    required_approvers  INTEGER NOT NULL DEFAULT 1,
    approvals_count     INTEGER NOT NULL DEFAULT 0,
    approver_permission TEXT NOT NULL DEFAULT 'approvals.decide',
    expires_at          TEXT NOT NULL,
    decided_at          TEXT,
    executed_at         TEXT,
    result_status       INTEGER,
    result_code         TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_approval_requests_status ON approval_requests(status, created_at);
CREATE INDEX IF NOT EXISTS idx_approval_requests_requester ON approval_requests(requester_id, status);
CREATE INDEX IF NOT EXISTS idx_approval_requests_rule ON approval_requests(rule_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_dedupe ON approval_requests(requester_id, method, path, status);

CREATE TABLE IF NOT EXISTS approval_decisions (
    id            TEXT PRIMARY KEY,
    request_id    TEXT NOT NULL,
    approver_id   TEXT NOT NULL,
    approver_name TEXT NOT NULL DEFAULT '',
    decision      TEXT NOT NULL,
    note          TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (request_id, approver_id),
    FOREIGN KEY (request_id) REFERENCES approval_requests(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_approval_decisions_request ON approval_decisions(request_id);
CREATE INDEX IF NOT EXISTS idx_approval_decisions_approver ON approval_decisions(approver_id);

CREATE TABLE IF NOT EXISTS break_glass_accounts (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL UNIQUE,
    max_minutes   INTEGER NOT NULL DEFAULT 60,
    note          TEXT NOT NULL DEFAULT '',
    designated_by TEXT,
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS break_glass_sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    reason      TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    ended_at    TEXT,
    ended_by    TEXT,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_break_glass_sessions_user ON break_glass_sessions(user_id, ended_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_break_glass_sessions_started ON break_glass_sessions(started_at);

INSERT INTO role_presets (id, key, name, description, permissions)
VALUES
    ('preset_viewer',
     'viewer',
     'Viewer',
     'Read-only access to workloads, environments, logs and dashboards. No secrets, no mutations.',
     '["account.self.manage","activity.view","alerts.view","autoheal.view","backup_log.view","backups.view","cluster.view","containers.view","dashboard.view","environments.view","events.view","host.view","images.view","in_app_notifications.view","incidents.view","logs.view","metrics.view","networks.view","reconciler.view","scans.view","schedules.view","stacks.view","updates.view","volumes.view","widgets.view","workflows.view"]'),

    ('preset_operator',
     'operator',
     'Operator',
     'Run day-to-day workloads: start, stop, deploy, back up and restore. No deletes, no settings, no identity.',
     '["account.self.manage","activity.view","alerts.manage","alerts.view","autoheal.manage","autoheal.view","backup_log.view","backups.manage","backups.view","cluster.view","containers.manage","containers.view","dashboard.view","environments.view","events.view","host.view","host_updates.view","images.manage","images.view","in_app_notifications.view","incidents.manage","incidents.view","logs.view","metrics.view","networks.manage","networks.view","reconciler.manage","reconciler.view","restore.run","restore.view","scans.view","schedules.manage","schedules.view","secrets.apply","secrets.view","stacks.manage","stacks.view","terminal.access","updates.manage","updates.view","volumes.manage","volumes.view","widgets.view","workflows.run","workflows.view"]'),

    ('preset_developer',
     'developer',
     'Developer',
     'Build and ship: stacks, images, Git, GitOps, workflows and their own API keys. No deletes, no settings, no identity.',
     '["account.self.manage","activity.view","ai_assistant.use","ai_assistant.view","alerts.view","api_keys.manage","autoheal.view","backup_log.view","backups.view","cluster.view","containers.manage","containers.view","custom_nodes.manage","custom_nodes.view","dashboard.view","environments.view","events.view","git.manage","git.view","gitops.manage","gitops.view","host.view","images.manage","images.view","in_app_notifications.view","incidents.view","logs.view","metrics.view","networks.manage","networks.view","reconciler.view","registry.view","scans.view","schedules.view","secrets.manage","secrets.view","stacks.manage","stacks.view","store_apps.view","terminal.access","updates.view","volumes.manage","volumes.view","widgets.view","workflows.manage","workflows.run","workflows.view"]'),

    ('preset_auditor',
     'auditor',
     'Auditor',
     'Read every governance surface and export the audit log. No mutations and no secret values.',
     '["account.self.manage","activity.view","api_keys.view","approvals.view","audit.export","audit.view","backup_log.view","cluster.view","containers.view","dashboard.view","environments.view","events.view","groups.view","identity_providers.view","images.view","in_app_notifications.view","incidents.view","networks.view","permissions.view","rbac.policies.view","roles.view","scans.view","scim.view","secrets.view","sessions.view","settings.view","stacks.view","users.view","volumes.view","widgets.view"]'),

    ('preset_security_admin',
     'security_admin',
     'Security Admin',
     'Users, groups, roles, policies, identity, API keys, sessions and approval rules. No workload control.',
     '["account.manage","account.password.reset","account.self.manage","activity.view","api_keys.manage","api_keys.view","approvals.decide","approvals.manage","approvals.view","audit.export","audit.view","dashboard.view","groups.manage","groups.view","identity_providers.manage","identity_providers.view","in_app_notifications.view","permissions.assign","permissions.view","rbac.policies.manage","rbac.policies.view","roles.manage","roles.view","scans.manage","scans.view","scim.manage","scim.view","secrets.view","sessions.manage","sessions.view","settings.view","users.manage","users.view","widgets.view"]'),

    ('preset_platform_admin',
     'platform_admin',
     'Platform Admin',
     'Every platform and workload action, but not identity, roles, policies or approval decisions, so running and governing the install stay two people.',
     '["account.self.manage","activity.view","ai_assistant.act","ai_assistant.manage","ai_assistant.use","ai_assistant.view","alerts.delete","alerts.manage","alerts.view","api_keys.manage","api_keys.view","approvals.view","audit.view","autoheal.manage","autoheal.view","backup_log.view","backups.delete","backups.download","backups.manage","backups.view","cluster.manage","cluster.view","communications.manage","communications.view","compliance.view","containers.delete","containers.manage","containers.view","custom_nodes.manage","custom_nodes.view","dashboard.view","database.manage","database.view","environments.manage","environments.view","events.view","git.manage","git.view","gitops.approve","gitops.delete","gitops.manage","gitops.view","groups.view","host.manage","host.view","host_updates.manage","host_updates.view","identity_providers.view","images.delete","images.manage","images.view","in_app_notifications.view","incidents.manage","incidents.view","instance_backup.manage","instance_backup.restore","instance_backup.view","license.view","logs.view","metrics.scrape","metrics.view","networks.delete","networks.manage","networks.view","permissions.view","plugins.delete","plugins.manage","plugins.view","rbac.policies.view","reconciler.enforce","reconciler.manage","reconciler.view","registry.manage","registry.view","restore.run","restore.view","roles.view","scans.manage","scans.view","schedules.delete","schedules.manage","schedules.view","scim.view","secrets.apply","secrets.delete","secrets.manage","secrets.manage_global","secrets.view","sessions.view","settings.manage","settings.view","stacks.delete","stacks.manage","stacks.view","storage_locations.delete","storage_locations.manage","storage_locations.view","store_apps.manage","store_apps.view","store_nodes.manage","store_nodes.view","store_widgets.manage","store_widgets.view","system.manage","terminal.access","updates.manage","updates.view","users.view","volumes.delete","volumes.manage","volumes.view","webhooks.delete","webhooks.manage","webhooks.view","widgets.manage","widgets.view","workflows.delete","workflows.manage","workflows.run","workflows.view"]')
ON CONFLICT (id) DO NOTHING;
