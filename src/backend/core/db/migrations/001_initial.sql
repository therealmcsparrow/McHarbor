-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- McHarbor Initial Schema
-- Mirrors the Drizzle ORM schema from the SvelteKit version

-- ─── Environments ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS environments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    connection_type TEXT NOT NULL DEFAULT 'socket' CHECK(connection_type IN ('socket','tcp','tls','ssh','podman')),
    socket_path TEXT,
    host TEXT,
    port INTEGER,
    tls_ca TEXT,
    tls_cert TEXT,
    tls_key TEXT,
    ssh_host TEXT,
    ssh_port INTEGER,
    ssh_user TEXT,
    ssh_key TEXT,
    is_default INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER NOT NULL DEFAULT 1,
    docker_version TEXT,
    last_connected TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_environments_default ON environments(is_default);

-- ─── Users ────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    email TEXT,
    password_hash TEXT NOT NULL,
    display_name TEXT,
    avatar TEXT,
    role TEXT DEFAULT 'admin',
    auth_provider TEXT NOT NULL DEFAULT 'local',
    is_active INTEGER NOT NULL DEFAULT 1,
    last_login TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- ─── Sessions ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL DEFAULT 'local',
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_expires ON sessions(user_id, expires_at);

-- ─── Auth Settings ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS auth_settings (
    id TEXT PRIMARY KEY,
    auth_enabled INTEGER NOT NULL DEFAULT 1,
    default_provider TEXT NOT NULL DEFAULT 'local',
    session_timeout INTEGER NOT NULL DEFAULT 86400,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Roles ────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    permissions TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── User Roles ───────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS user_roles (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);

-- ─── Registries ───────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS registries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    username TEXT,
    password TEXT,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Settings ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS settings (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT,
    category TEXT DEFAULT 'general',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Stacks ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS stacks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    project_path TEXT NOT NULL,
    compose_file TEXT NOT NULL DEFAULT 'docker-compose.yml',
    status TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('running','stopped','partial','unknown')),
    description TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_stacks_env ON stacks(environment_id);

-- ─── Stack Environment Variables ──────────────────────────────────────
CREATE TABLE IF NOT EXISTS stack_environment_variables (
    id TEXT PRIMARY KEY,
    stack_id TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT,
    is_secret INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_stack_env_vars_stack ON stack_environment_variables(stack_id);

-- ─── Stack Sources ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS stack_sources (
    id TEXT PRIMARY KEY,
    stack_id TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK(source_type IN ('file','git','url','paste')),
    source_path TEXT,
    git_url TEXT,
    git_branch TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Container Events ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS container_events (
    id TEXT PRIMARY KEY,
    environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_id TEXT NOT NULL,
    container_name TEXT,
    event_type TEXT NOT NULL,
    action TEXT NOT NULL,
    metadata TEXT,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_container_events_env ON container_events(environment_id);
CREATE INDEX IF NOT EXISTS idx_container_events_timestamp ON container_events(timestamp);

-- ─── Host Metrics ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS host_metrics (
    id TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    cpu_percent INTEGER,
    memory_percent INTEGER,
    memory_used INTEGER,
    memory_total INTEGER,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_host_metrics_env_time ON host_metrics(environment_id, timestamp);

-- ─── Audit Logs ───────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    user_id TEXT,
    username TEXT,
    action TEXT NOT NULL,
    entity_type TEXT,
    entity_id TEXT,
    entity_name TEXT,
    details TEXT,
    ip_address TEXT,
    environment_id TEXT,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs(entity_type, entity_id);

-- ─── Notification Channels ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS notification_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('webhook','email','slack','discord','telegram')),
    config TEXT NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Environment Notifications ────────────────────────────────────────
CREATE TABLE IF NOT EXISTS environment_notifications (
    id TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    notification_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    events TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_env_notifications_env ON environment_notifications(environment_id);

-- ─── Schedules ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS schedules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    cron TEXT NOT NULL,
    action TEXT NOT NULL,
    target TEXT,
    env_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at TEXT,
    next_run_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Schedule Executions ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS schedule_executions (
    id TEXT PRIMARY KEY,
    schedule_id TEXT NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK(status IN ('success','failure','running')),
    output TEXT,
    duration INTEGER NOT NULL DEFAULT 0,
    executed_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_schedule_exec_schedule ON schedule_executions(schedule_id);

-- ─── Blueprints ──────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS blueprints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    category TEXT NOT NULL DEFAULT 'other',
    compose_yaml TEXT NOT NULL DEFAULT '',
    env_vars TEXT DEFAULT '[]',
    icon TEXT,
    version TEXT DEFAULT '1.0.0',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_blueprints_category ON blueprints(category);

-- ─── Webhooks ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret TEXT,
    events TEXT NOT NULL DEFAULT '[]',
    is_active INTEGER NOT NULL DEFAULT 1,
    environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    headers TEXT DEFAULT '{}',
    max_retries INTEGER NOT NULL DEFAULT 3,
    timeout_seconds INTEGER NOT NULL DEFAULT 10,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    response_status INTEGER,
    response_body TEXT,
    success INTEGER NOT NULL DEFAULT 0,
    duration INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id);

-- ─── Desired State / Reconciler ──────────────────────────────────────
CREATE TABLE IF NOT EXISTS desired_states (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    env_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_name TEXT NOT NULL DEFAULT '',
    image_ref TEXT NOT NULL DEFAULT '',
    desired_status TEXT NOT NULL DEFAULT 'running',
    restart_policy TEXT NOT NULL DEFAULT 'unless-stopped',
    config TEXT DEFAULT '{}',
    last_reconcile TEXT,
    drift_detected INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_desired_states_env ON desired_states(env_id);

CREATE TABLE IF NOT EXISTS reconciliation_logs (
    id TEXT PRIMARY KEY,
    desired_state_id TEXT NOT NULL REFERENCES desired_states(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('success','failure','skipped')),
    details TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reconciliation_logs_state ON reconciliation_logs(desired_state_id);

-- ─── Alerts ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    condition TEXT,
    target TEXT,
    channel_id TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ─── Vulnerability Scans ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS scans (
    id TEXT PRIMARY KEY,
    image_ref TEXT NOT NULL,
    scanner TEXT NOT NULL DEFAULT 'trivy',
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','completed','failed')),
    severity TEXT,
    total_vulns INTEGER NOT NULL DEFAULT 0,
    critical_count INTEGER NOT NULL DEFAULT 0,
    high_count INTEGER NOT NULL DEFAULT 0,
    medium_count INTEGER NOT NULL DEFAULT 0,
    low_count INTEGER NOT NULL DEFAULT 0,
    started_at TEXT,
    completed_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_scans_image ON scans(image_ref);

CREATE TABLE IF NOT EXISTS scan_vulnerabilities (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    vuln_id TEXT NOT NULL,
    pkg_name TEXT NOT NULL,
    pkg_version TEXT,
    fixed_version TEXT,
    severity TEXT NOT NULL CHECK(severity IN ('critical','high','medium','low','negligible','unknown')),
    title TEXT,
    description TEXT,
    url TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_scan_vulns_scan ON scan_vulnerabilities(scan_id);

-- ─── Auto Updates ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS update_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    container_match TEXT NOT NULL DEFAULT '*',
    image_match TEXT NOT NULL DEFAULT '*',
    schedule TEXT NOT NULL DEFAULT '0 3 * * *',
    strategy TEXT NOT NULL DEFAULT 'latest' CHECK(strategy IN ('latest','semver','digest')),
    auto_restart INTEGER NOT NULL DEFAULT 1,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at TEXT,
    last_run_status TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS update_history (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES update_policies(id) ON DELETE CASCADE,
    container TEXT NOT NULL,
    old_image TEXT NOT NULL,
    new_image TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('success','failure','skipped','rolled_back')),
    message TEXT,
    executed_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_update_history_policy ON update_history(policy_id);

-- ─── Git Repositories ────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS git_repos (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    path TEXT DEFAULT '',
    auth_type TEXT NOT NULL DEFAULT 'none',
    credential_ref TEXT DEFAULT '',
    auto_sync INTEGER NOT NULL DEFAULT 0,
    sync_interval INTEGER NOT NULL DEFAULT 300,
    env_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    last_sync_at TEXT,
    last_sync_error TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS git_deployments (
    id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    commit_sha TEXT NOT NULL,
    branch TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending','deploying','success','failure','rolled_back')),
    message TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_git_deployments_repo ON git_deployments(repo_id);

-- ─── Plugins ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS plugins (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    version TEXT NOT NULL DEFAULT '1.0.0',
    author TEXT,
    source TEXT NOT NULL DEFAULT '',
    config TEXT DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 0,
    installed_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
