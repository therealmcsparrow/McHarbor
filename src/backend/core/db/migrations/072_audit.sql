-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Audit: the columns that make a log row tamper-evident, WORM triggers,
-- retention, the action catalog and route map, and log forwarders.

ALTER TABLE audit_logs
    ADD COLUMN severity TEXT NOT NULL DEFAULT 'info'
    CHECK(severity IN ('info','success','warning','error'));

UPDATE audit_logs
SET severity = CASE
    WHEN action IN ('delete', 'remove', 'purge', 'revoke', 'disable', 'stop', 'kill') THEN 'warning'
    WHEN action LIKE '%failed%' OR action LIKE '%error%' OR action LIKE '%denied%' THEN 'error'
    WHEN action IN ('create', 'start', 'enable', 'assign', 'login') THEN 'success'
    ELSE 'info'
END;

CREATE INDEX IF NOT EXISTS idx_audit_logs_severity_time
    ON audit_logs(severity, timestamp DESC);


ALTER TABLE audit_logs
    ADD COLUMN state_before TEXT;

ALTER TABLE audit_logs
    ADD COLUMN state_after TEXT;

ALTER TABLE audit_logs
    ADD COLUMN changed_keys TEXT;


ALTER TABLE audit_logs ADD COLUMN prev_hash TEXT;
ALTER TABLE audit_logs ADD COLUMN row_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_logs_row_hash ON audit_logs(row_hash);


ALTER TABLE audit_logs ADD COLUMN environment_name TEXT;

UPDATE audit_logs
SET environment_name = (
    SELECT e.name FROM environments e WHERE e.id = audit_logs.environment_id
)
WHERE environment_id IS NOT NULL
  AND environment_id != ''
  AND environment_name IS NULL;


INSERT INTO settings (id, key, value, category, created_at, updated_at)
VALUES ('setting_audit_worm', 'audit.worm', 'off', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO NOTHING;

INSERT INTO settings (id, key, value, category, created_at, updated_at)
VALUES ('setting_audit_worm_delete_allowed', 'audit.worm_delete_allowed', '0', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO NOTHING;

CREATE TRIGGER IF NOT EXISTS trg_audit_logs_worm_no_update
BEFORE UPDATE ON audit_logs
WHEN COALESCE((SELECT value FROM settings WHERE key = 'audit.worm'), 'off') = 'on'
BEGIN
    SELECT RAISE(ABORT, 'audit_logs WORM protection blocks updates');
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_logs_worm_no_delete
BEFORE DELETE ON audit_logs
WHEN COALESCE((SELECT value FROM settings WHERE key = 'audit.worm'), 'off') = 'on'
     AND COALESCE((SELECT value FROM settings WHERE key = 'audit.worm_delete_allowed'), '0') != '1'
BEGIN
    SELECT RAISE(ABORT, 'audit_logs WORM protection blocks deletes');
END;


CREATE TABLE IF NOT EXISTS audit_retention_presets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    framework TEXT NOT NULL,
    retention_days INTEGER NOT NULL CHECK(retention_days >= 0 AND retention_days <= 3650),
    description TEXT NOT NULL DEFAULT '',
    is_custom INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_retention_presets_sort ON audit_retention_presets(sort_order, name);

INSERT INTO audit_retention_presets (id, name, framework, retention_days, description, is_custom, sort_order, created_at, updated_at)
VALUES
    ('soc2', 'SOC 2', 'SOC 2', 365, 'Keep audit logs for one year for SOC 2 evidence review.', 0, 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('hipaa', 'HIPAA', 'HIPAA', 2190, 'Keep audit logs for six years for HIPAA administrative safeguards.', 0, 20, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('sox', 'SOX', 'SOX', 2555, 'Keep audit logs for seven years for SOX financial control evidence.', 0, 30, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('pci_dss', 'PCI DSS', 'PCI DSS', 365, 'Keep audit logs for one year for PCI DSS audit-trail review.', 0, 40, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('fisma', 'FISMA', 'FISMA', 1095, 'Keep audit logs for three years for FISMA security monitoring evidence.', 0, 50, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('custom', 'Custom', 'Custom', 90, 'Use the manually configured audit retention value.', 1, 999, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    framework = excluded.framework,
    retention_days = excluded.retention_days,
    description = excluded.description,
    is_custom = excluded.is_custom,
    sort_order = excluded.sort_order,
    updated_at = CURRENT_TIMESTAMP;

INSERT INTO settings (id, key, value, category, created_at, updated_at)
VALUES ('setting_retention_audit_preset', 'retention_audit_preset', 'custom', 'retention', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO NOTHING;


CREATE TABLE IF NOT EXISTS audit_action_catalog (
  id TEXT PRIMARY KEY,
  module TEXT NOT NULL,
  action TEXT NOT NULL UNIQUE,
  severity_default TEXT NOT NULL CHECK (severity_default IN ('info', 'success', 'warning', 'error')),
  description TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_action_catalog_module ON audit_action_catalog(module);
CREATE INDEX IF NOT EXISTS idx_audit_action_catalog_action ON audit_action_catalog(action);

CREATE VIEW IF NOT EXISTS audit_logs_by_plugin AS
SELECT
  entity_id AS plugin_slug,
  COUNT(1) AS count,
  MAX(timestamp) AS last_seen,
  SUM(CASE WHEN severity = 'info' THEN 1 ELSE 0 END) AS info_count,
  SUM(CASE WHEN severity = 'success' THEN 1 ELSE 0 END) AS success_count,
  SUM(CASE WHEN severity = 'warning' THEN 1 ELSE 0 END) AS warning_count,
  SUM(CASE WHEN severity = 'error' THEN 1 ELSE 0 END) AS error_count
FROM audit_logs
WHERE entity_type = 'plugin' AND entity_id IS NOT NULL AND entity_id != ''
GROUP BY entity_id;

INSERT OR IGNORE INTO audit_action_catalog (id, module, action, severity_default, description) VALUES
('audit_action_001', 'auth', 'login', 'success', 'User signed in successfully'),
('audit_action_002', 'auth', 'logout', 'info', 'User signed out'),
('audit_action_003', 'auth', 'login_failed', 'error', 'User sign-in attempt failed'),
('audit_action_004', 'auth', 'setup.completed', 'success', 'Initial application setup completed'),
('audit_action_005', 'audit', 'audit.accessed', 'info', 'Audit log was viewed or searched'),
('audit_action_006', 'audit', 'audit.exported', 'success', 'Audit log export was generated'),
('audit_action_007', 'audit', 'audit.verified', 'success', 'Audit hash chain verification was requested'),
('audit_action_008', 'audit', 'audit.verified.scheduled', 'success', 'Scheduled audit hash chain verification completed'),
('audit_action_009', 'audit', 'purge', 'warning', 'Expired audit rows were purged by retention policy'),
('audit_action_010', 'audit', 'audit.siem.config.updated', 'success', 'SIEM forwarding configuration changed'),
('audit_action_011', 'audit', 'audit.siem.test.sent', 'success', 'SIEM forwarding test event sent'),
('audit_action_012', 'audit', 'audit.syslog.config.updated', 'success', 'Syslog forwarding configuration changed'),
('audit_action_013', 'audit', 'audit.syslog.test.sent', 'success', 'Syslog forwarding test event sent'),
('audit_action_014', 'clock', 'clock.boot_check', 'info', 'Clock state was recorded during application boot'),
('audit_action_015', 'clock', 'clock.drift_check', 'success', 'System clock was compared with the configured UTC reference'),
('audit_action_016', 'containers', 'start', 'success', 'Container was started'),
('audit_action_017', 'containers', 'stop', 'warning', 'Container was stopped'),
('audit_action_018', 'containers', 'restart', 'warning', 'Container was restarted'),
('audit_action_019', 'containers', 'kill', 'warning', 'Container was killed'),
('audit_action_020', 'containers', 'remove', 'warning', 'Container was removed'),
('audit_action_021', 'containers', 'create', 'success', 'Container was created'),
('audit_action_022', 'images', 'pull', 'success', 'Image was pulled'),
('audit_action_023', 'images', 'delete', 'warning', 'Image was deleted'),
('audit_action_024', 'volumes', 'volume.created', 'success', 'Volume was created'),
('audit_action_025', 'volumes', 'volume.removed', 'warning', 'Volume was removed'),
('audit_action_026', 'networks', 'network.created', 'success', 'Network was created'),
('audit_action_027', 'networks', 'network.removed', 'warning', 'Network was removed'),
('audit_action_028', 'stacks', 'stack.created', 'success', 'Stack was created'),
('audit_action_029', 'stacks', 'stack.updated', 'success', 'Stack was updated'),
('audit_action_030', 'stacks', 'stack.deleted', 'warning', 'Stack was deleted'),
('audit_action_031', 'stacks', 'stack.deployed', 'success', 'Stack was deployed'),
('audit_action_032', 'environments', 'environment.created', 'success', 'Environment was created'),
('audit_action_033', 'environments', 'environment.updated', 'success', 'Environment was updated'),
('audit_action_034', 'environments', 'environment.deleted', 'warning', 'Environment was deleted'),
('audit_action_035', 'users', 'user.created', 'success', 'User was created'),
('audit_action_036', 'users', 'user.updated', 'success', 'User was updated'),
('audit_action_037', 'users', 'user.deleted', 'warning', 'User was deleted'),
('audit_action_038', 'groups', 'group.created', 'success', 'Group was created'),
('audit_action_039', 'groups', 'group.updated', 'success', 'Group was updated'),
('audit_action_040', 'groups', 'group.deleted', 'warning', 'Group was deleted'),
('audit_action_041', 'plugins', 'plugin.install', 'success', 'Plugin was installed'),
('audit_action_042', 'plugins', 'plugin.uninstall', 'warning', 'Plugin was uninstalled'),
('audit_action_043', 'plugins', 'plugin.update', 'success', 'Plugin was updated'),
('audit_action_044', 'plugins', 'plugin.error', 'error', 'Plugin emitted an error audit event'),
('audit_action_045', 'workflows', 'workflow.created', 'success', 'Workflow was created'),
('audit_action_046', 'workflows', 'workflow.updated', 'success', 'Workflow was updated'),
('audit_action_047', 'workflows', 'workflow.deleted', 'warning', 'Workflow was deleted'),
('audit_action_048', 'workflows', 'workflow.executed', 'success', 'Workflow was executed'),
('audit_action_049', 'terminal', 'terminal.exec', 'warning', 'Container terminal session was opened'),
('audit_action_050', 'settings', 'settings.updated', 'success', 'Application setting was changed');


INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES
  ('audit_hash_chain_enabled', 'audit.hash_chain.enabled', 'true', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  ('audit_action_catalog_enabled', 'audit.action_catalog.enabled', 'true', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  ('audit_siem_forwarding_enabled', 'audit.siem_forwarding.enabled', 'true', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  ('audit_syslog_forwarding_enabled', 'audit.syslog_forwarding.enabled', 'true', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);


INSERT OR IGNORE INTO settings (id, key, value, category, created_at, updated_at)
VALUES ('audit_clock_reference_url', 'audit.clock.reference_url', 'https://www.cloudflare.com/', 'audit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);


CREATE TABLE IF NOT EXISTS audit_action_route_map (
  id TEXT PRIMARY KEY,
  module TEXT NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  handler TEXT NOT NULL,
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id_source TEXT NOT NULL DEFAULT '',
  audit_required INTEGER NOT NULL DEFAULT 1,
  rationale TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(method, path, handler)
);

CREATE INDEX IF NOT EXISTS idx_audit_action_route_map_module ON audit_action_route_map(module);
CREATE INDEX IF NOT EXISTS idx_audit_action_route_map_action ON audit_action_route_map(action);
CREATE INDEX IF NOT EXISTS idx_audit_action_route_map_required ON audit_action_route_map(audit_required);

INSERT OR IGNORE INTO audit_action_catalog (id, module, action, severity_default, description) VALUES
('audit_action_051', 'api_keys', 'api_key.created', 'success', 'API key was created or revealed once'),
('audit_action_052', 'api_keys', 'api_key.revoked', 'warning', 'API key was revoked'),
('audit_action_053', 'agent', 'agent.token.regenerated', 'warning', 'Remote agent token was regenerated'),
('audit_action_054', 'auth', 'auth.denied', 'warning', 'Privileged request was denied'),
('audit_action_055', 'identity', 'identity_provider.callback', 'info', 'Identity provider callback completed'),
('audit_action_056', 'custom_nodes', 'custom_node.created', 'success', 'Custom workflow node was created'),
('audit_action_057', 'custom_nodes', 'custom_node.updated', 'success', 'Custom workflow node was updated'),
('audit_action_058', 'custom_nodes', 'custom_node.deleted', 'warning', 'Custom workflow node was deleted'),
('audit_action_059', 'custom_nodes', 'custom_node.tested', 'warning', 'Custom workflow node script was tested'),
('audit_action_060', 'custom_nodes', 'custom_node.runtime_failed', 'error', 'Custom workflow node failed during runtime'),
('audit_action_061', 'plugins', 'plugin.configure', 'success', 'Plugin configuration was changed'),
('audit_action_062', 'plugins', 'plugin.route.fallback', 'warning', 'Host fallback audit was recorded for a plugin-owned route'),
('audit_action_063', 'workflows', 'workflow.imported', 'success', 'Workflow was imported'),
('audit_action_064', 'workflows', 'workflow.exported', 'success', 'Workflow was exported'),
('audit_action_065', 'workflows', 'workflow.enabled', 'success', 'Workflow was enabled'),
('audit_action_066', 'workflows', 'workflow.disabled', 'warning', 'Workflow was disabled'),
('audit_action_067', 'settings', 'settings.secret_metadata.accessed', 'info', 'Secret or credential metadata was accessed'),
('audit_action_068', 'audit', 'audit.coverage.accessed', 'info', 'Audit coverage report was viewed'),
('audit_action_069', 'audit', 'audit.coverage.exported', 'success', 'Audit coverage report was exported'),
('audit_action_070', 'agent', 'agent.install_script.accessed', 'warning', 'Remote agent install script was retrieved with a one-time token');

INSERT OR IGNORE INTO audit_action_route_map (id, module, method, path, handler, action, entity_type, entity_id_source, audit_required, rationale) VALUES
('audit_route_001', 'audit', 'GET', '/audit', 'HandleList', 'audit.accessed', 'audit_log', 'query', 1, 'Sensitive audit log access'),
('audit_route_002', 'audit', 'GET', '/audit/export', 'HandleExport', 'audit.exported', 'audit_log', 'query', 1, 'Compliance evidence export'),
('audit_route_003', 'audit', 'GET', '/audit/verify', 'HandleVerify', 'audit.verified', 'audit_log', 'hash_chain', 1, 'Audit integrity verification'),
('audit_route_004', 'audit', 'GET', '/audit/coverage', 'HandleCoverage', 'audit.coverage.accessed', 'audit_coverage', 'route_map', 1, 'Coverage evidence access'),
('audit_route_005', 'audit', 'GET', '/audit/coverage/export', 'HandleCoverageExport', 'audit.coverage.exported', 'audit_coverage', 'route_map', 1, 'Coverage evidence export'),
('audit_route_006', 'api_keys', 'POST', '/api-keys', 'HandleCreate', 'api_key.created', 'api_key', 'response.id', 1, 'Reveal-once credential creation'),
('audit_route_007', 'api_keys', 'DELETE', '/api-keys/{id}', 'HandleRevoke', 'api_key.revoked', 'api_key', 'path.id', 1, 'Credential revocation'),
('audit_route_008', 'agent', 'POST', '/agents/{envId}/regenerate-token', 'HandleRegenerateToken', 'agent.token.regenerated', 'agent', 'path.envId', 1, 'Agent token rotation'),
('audit_route_009', 'custom_nodes', 'POST', '/custom-nodes', 'HandleCreate', 'custom_node.created', 'custom_node', 'request.key', 1, 'Custom script creation'),
('audit_route_010', 'custom_nodes', 'PUT', '/custom-nodes/{key}', 'HandleUpdate', 'custom_node.updated', 'custom_node', 'path.key', 1, 'Custom script update'),
('audit_route_011', 'custom_nodes', 'DELETE', '/custom-nodes/{key}', 'HandleDelete', 'custom_node.deleted', 'custom_node', 'path.key', 1, 'Custom script deletion'),
('audit_route_012', 'custom_nodes', 'POST', '/custom-nodes/test', 'HandleTest', 'custom_node.tested', 'custom_node', 'request.key', 1, 'Sandboxed custom script execution'),
('audit_route_013', 'plugins', 'POST', '/appstore/plugins/{slug}/install', 'HandleInstall', 'plugin.install', 'plugin', 'path.slug', 1, 'Plugin lifecycle mutation'),
('audit_route_014', 'plugins', 'DELETE', '/appstore/plugins/{slug}', 'HandleUninstall', 'plugin.uninstall', 'plugin', 'path.slug', 1, 'Plugin lifecycle mutation'),
('audit_route_015', 'plugins', 'PUT', '/appstore/plugins/{slug}/config', 'HandleConfigure', 'plugin.configure', 'plugin', 'path.slug', 1, 'Plugin configuration mutation'),
('audit_route_016', 'workflows', 'POST', '/workflows', 'HandleCreate', 'workflow.created', 'workflow', 'response.id', 1, 'Workflow definition creation'),
('audit_route_017', 'workflows', 'PUT', '/workflows/{id}', 'HandleUpdate', 'workflow.updated', 'workflow', 'path.id', 1, 'Workflow definition update'),
('audit_route_018', 'workflows', 'DELETE', '/workflows/{id}', 'HandleDelete', 'workflow.deleted', 'workflow', 'path.id', 1, 'Workflow definition deletion'),
('audit_route_019', 'workflows', 'POST', '/workflows/{id}/execute', 'HandleExecute', 'workflow.executed', 'workflow', 'path.id', 1, 'Workflow execution'),
('audit_route_020', 'workflows', 'POST', '/workflows/import', 'HandleImport', 'workflow.imported', 'workflow', 'response.id', 1, 'Workflow import'),
('audit_route_021', 'workflows', 'GET', '/workflows/{id}/export', 'HandleExport', 'workflow.exported', 'workflow', 'path.id', 1, 'Workflow export');


CREATE TABLE IF NOT EXISTS audit_log_forwarders (
    id              TEXT PRIMARY KEY,
    -- Operator-facing label. Seeded from the type for rows that
    -- predate this column, which is the honest name for a forwarder
    -- created when a type could only exist once.
    name            TEXT NOT NULL,
    forwarder_type  TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0, 1)),
    config          TEXT NOT NULL DEFAULT '{}',
    -- enabled_since is the per-instance watermark: a second collector
    -- added today starts from today, rather than inheriting the first
    -- one's history.
    enabled_since   TEXT,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_audit_log_forwarders_enabled
    ON audit_log_forwarders(enabled, forwarder_type);


-- Action catalog entries for the plugin-backed log forwarder surface.
--
-- The two hard-coded transports each emitted their own actions
-- (audit.siem.config.updated, audit.syslog.test.sent, and siblings,
-- seeded with the action catalog above). With forwarding behind one generic
-- admin surface there are fewer actions, carrying the forwarder type as the
-- entity id rather than in the action name.
--
-- The old rows are deliberately left in place: audit_logs may still hold
-- events with those actions, and deleting a catalog entry would strip the
-- severity and description off historical rows in the Compliance view. A
-- catalog is an append-only vocabulary.
--
-- Severity: saving is `warning` because changing where an audit trail
-- is shipped is exactly the configuration change an auditor wants
-- flagged — including switching it off. Testing is `info`: it proves
-- reachability and changes nothing.
--
-- The conflict target is `action`, which is the column that actually carries
-- a UNIQUE constraint and the one that expresses the intent: catalogue this
-- action unless it is already catalogued.

INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES
  ('audit_action_071', 'audit', 'audit.forwarder.saved',   'warning', 'Audit log forwarder configuration changed'),
  ('audit_action_072', 'audit', 'audit.forwarder.tested',  'info',    'Audit log forwarder test event sent'),
  ('audit_action_073', 'audit', 'audit.forwarder.deleted', 'warning', 'Audit log forwarder removed')
ON CONFLICT(action) DO NOTHING;
