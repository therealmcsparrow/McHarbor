-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- RBAC v2: the permission catalog, per-user grants, policies, presets,
-- elevation, resource ownership, and every seed and backfill that goes
-- with them.

-- ─── 1. Permission catalog (server is source of truth) ─────────────────
--
-- is_elevatable: which permissions can be requested through a time-bound
-- elevation. rbac.UpsertCatalog refreshes category / action / description /
-- scope at every boot but never is_elevatable, so the rows seeded here carry
-- the flag already set.
CREATE TABLE IF NOT EXISTS permission_catalog (
    key          TEXT PRIMARY KEY,            -- e.g. 'containers.manage'
    category     TEXT NOT NULL,               -- e.g. 'containers'
    action       TEXT NOT NULL,               -- e.g. 'manage'
    description  TEXT NOT NULL,
    scope        TEXT NOT NULL CHECK (scope IN ('global','environment','stack','owner','account')),
    is_system    INTEGER NOT NULL DEFAULT 1,
    added_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    is_elevatable INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_permission_catalog_category ON permission_catalog(category);

-- rbac_policy_bindings has a FK on permission_key → permission_catalog, so
-- the permissions referenced by the seed bindings must exist before they are
-- inserted. The app's UpsertCatalog() call would do this later at startup,
-- but migrations run before that.
--
-- backups.download is deliberately left non-elevatable so it does not add a
-- step-up prompt to an operation that had none; flip it to 1 if downloads
-- should require approval.
INSERT INTO permission_catalog (key, category, action, description, scope, is_system, is_elevatable)
VALUES
    ('containers.manage', 'containers', 'manage',   'Manage containers (start, stop, restart, exec)', 'environment', 1, 1),
    ('containers.exec',   'containers', 'exec',     'Execute into container shells',                  'environment', 1, 0),
    ('containers.delete', 'containers', 'delete',   'Delete/remove containers',                       'environment', 1, 1),
    ('backups.view',      'backups',    'view',     'View backup plans, runs, and the backups page',  'environment', 1, 0),
    ('backups.manage',    'backups',    'manage',   'Create / edit backup plans and run backups',     'owner',       1, 1),
    ('backups.delete',    'backups',    'delete',   'Delete backup plans and runs',                   'environment', 1, 1),
    ('backups.download',  'backups',    'download', 'Download a backup archive',                      'environment', 1, 0),
    ('restore.view',      'restore',    'view',     'Browse the contents of a backup archive',        'environment', 1, 0),
    ('restore.run',       'restore',    'run',      'Restore a backup into a container',              'environment', 1, 1)
ON CONFLICT (key) DO NOTHING;

-- ─── 2. User-level permission overrides (grant/deny) ───────────────────
CREATE TABLE IF NOT EXISTS user_permissions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    permission_key  TEXT NOT NULL,
    effect          TEXT NOT NULL CHECK (effect IN ('grant','deny')),
    environment_id  TEXT,                     -- NULL = global
    stack_name      TEXT,                     -- NULL = no stack scope
    resource_type   TEXT,                     -- e.g. 'container' (NULL = any)
    resource_id     TEXT,                     -- optional specific row
    granted_by      TEXT NOT NULL,
    expires_at      TEXT,                     -- NULL = never
    reason          TEXT,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_key) REFERENCES permission_catalog(key) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_user_permissions_user ON user_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_permissions_lookup ON user_permissions(user_id, permission_key);
CREATE INDEX IF NOT EXISTS idx_user_permissions_expires ON user_permissions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_permissions_permission_key ON user_permissions(permission_key);

-- ─── 3. Owner column on owned resources ────────────────────────────────
-- Each owned resource table gains an owner_user_id column. We use
-- nullable TEXT to allow unowned historical rows. The `containers`
-- table is not persisted in SQL (containers are read live from the
-- Docker daemon), so we do NOT alter it here — container ownership
-- is tracked via Docker labels.
--
-- Rows whose owner_user_id is still NULL are "legacy / unowned" and are
-- assigned to the `__legacy_owner__` placeholder (066_accounts.sql).

ALTER TABLE stacks ADD COLUMN owner_user_id TEXT;
ALTER TABLE container_backup_plans ADD COLUMN owner_user_id TEXT;

CREATE INDEX IF NOT EXISTS idx_stacks_owner ON stacks(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_container_backup_plans_owner ON container_backup_plans(owner_user_id);

UPDATE stacks
   SET owner_user_id = '__legacy_owner__'
 WHERE owner_user_id IS NULL;

UPDATE container_backup_plans
   SET owner_user_id = '__legacy_owner__'
 WHERE owner_user_id IS NULL;

-- ─── 4. RBAC v2 feature flag ───────────────────────────────────────────
-- Default-on for new installs, opt-in for upgrades. Upgrading instances
-- (those with any user rows) get 'false' so admins can audit and accept
-- the new permission model before flipping the switch in
-- Settings → Security → "RBAC v2 (fine-grained permissions)".
INSERT INTO settings (id, key, value, category)
    VALUES ('set_rbac_v2_enabled', 'rbac_v2_enabled', 'true', 'security')
    ON CONFLICT (id) DO NOTHING;

UPDATE settings
   SET value = 'false'
 WHERE key = 'rbac_v2_enabled'
   AND value = 'true'
   AND (SELECT COUNT(*) FROM users) > 0;


-- ABAC policy tables
--
-- Policies are admin-authored JavaScript expressions evaluated against
-- a JSON-shaped attribute bundle (user, env, resource). When a policy
-- matches, its effect (allow/deny) decides the Authorize() outcome for
-- the candidate permission before the role-based check runs.
--
-- A policy can be bound to:
--   * a permission (matches any request for that permission), and
--   * (optionally) a resource_type (e.g. 'container', 'stack'). An empty
--     resource_type matches any resource.
--
-- A policy is only evaluated when at least one binding row exists.
-- The seeded library further down inserts a few disabled rows so admins can
-- read and enable them after reviewing.
--
-- `origin` is the plugin slug for policies a plugin registers
-- (core/pluginruntime/loader.go InstallABACRule), so uninstall can drop them
-- with `WHERE origin = ?`. The seed policies are author-authored, not
-- plugin-authored, so they carry origin = '' and outlive plugin uninstalls.

-- ─── 1. Policies ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS rbac_policies (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    effect        TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    resource_type TEXT NOT NULL DEFAULT '',          -- '' = any resource
    expression    TEXT NOT NULL,                     -- JS expression
    priority      INTEGER NOT NULL DEFAULT 100,      -- lower = evaluated first
    enabled       INTEGER NOT NULL DEFAULT 0,
    created_by    TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    origin        TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_rbac_policies_enabled
    ON rbac_policies(enabled, priority);
CREATE INDEX IF NOT EXISTS idx_rbac_policies_resource
    ON rbac_policies(resource_type);
CREATE INDEX IF NOT EXISTS idx_rbac_policies_origin
    ON rbac_policies(origin);
CREATE INDEX IF NOT EXISTS idx_rbac_policies_created_by ON rbac_policies(created_by);

-- ─── 2. Policy ↔ permission bindings ────────────────────────────────
-- A policy matches a request when the candidate permission appears
-- here. Multiple permission rows per policy are allowed (one policy
-- may refine several permissions).
CREATE TABLE IF NOT EXISTS rbac_policy_bindings (
    id              TEXT PRIMARY KEY,
    policy_id       TEXT NOT NULL,
    permission_key  TEXT NOT NULL,
    FOREIGN KEY (policy_id) REFERENCES rbac_policies(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_key) REFERENCES permission_catalog(key) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_rbac_policy_bindings_policy
    ON rbac_policy_bindings(policy_id);
CREATE INDEX IF NOT EXISTS idx_rbac_policy_bindings_perm
    ON rbac_policy_bindings(permission_key);

-- ─── 3. Audit trail for policy changes ──────────────────────────────
CREATE TABLE IF NOT EXISTS rbac_policy_history (
    id          TEXT PRIMARY KEY,
    policy_id   TEXT NOT NULL,
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,                       -- 'create','update','delete','enable','disable'
    details     TEXT,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_rbac_policy_history_policy
    ON rbac_policy_history(policy_id, created_at);

-- ─── 4. Rollout flag (mirrors rbac_v2_enabled) ───────────────────────
-- Default-on for new installs (no users yet), default-off for upgrades.
INSERT INTO settings (id, key, value, category)
    VALUES ('set_rbac_v3_enabled', 'rbac_v3_enabled', 'true', 'security')
    ON CONFLICT (id) DO NOTHING;

UPDATE settings
   SET value = 'false'
 WHERE key = 'rbac_v3_enabled'
   AND value = 'true'
   AND (SELECT COUNT(*) FROM users) > 0;

-- ─── 5. Per-user labels (for ABAC seed policies) ────────────────────
-- Free-form key/value bag used by the "team ownership" seed policy
-- to match user.team to container.team. Stored as a simple table so
-- admins can update it via the user profile page.
CREATE TABLE IF NOT EXISTS user_labels (
    user_id    TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    PRIMARY KEY (user_id, key),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_labels_user ON user_labels(user_id);


-- Read-only seeded policy library
--
-- These policies are inserted on every install with enabled=0. Admins see
-- them in the UI and can flip them on after they have reviewed the
-- expression. The default-on would be risky on upgrades because
-- existing workloads would silently gain new restrictions.
--
-- Five seeds cover the most common patterns:
--   1. Production tier denied for `containers.manage`
--   2. PII containers denied for `containers.exec`
--   3. Read-only label honored for `containers.delete`
--   4. Only platforms team can `containers.manage` containers tagged team=platform
--   5. Off-hours require session time within window (deny if outside business hours)

-- ─── 1. Production is read-only ─────────────────────────────────────
INSERT INTO rbac_policies (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
VALUES (
    'seed_prod_readonly',
    'Production tier is read-only',
    'Refuses containers.manage when the container has the mcharbor.tier=prod label.',
    'deny',
    'container',
    'resource.labels["mcharbor.tier"] == "prod"',
    10,
    0,
    '__legacy_owner__'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO rbac_policy_bindings (id, policy_id, permission_key)
VALUES ('seed_pb_prod_rm', 'seed_prod_readonly', 'containers.manage')
ON CONFLICT (id) DO NOTHING;

-- ─── 2. PII containers deny exec ────────────────────────────────────
INSERT INTO rbac_policies (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
VALUES (
    'seed_pii_no_exec',
    'PII containers deny exec',
    'Refuses containers.manage (which includes exec) when the container has compliance.pii=true.',
    'deny',
    'container',
    'resource.labels["compliance.pii"] == "true"',
    10,
    0,
    '__legacy_owner__'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO rbac_policy_bindings (id, policy_id, permission_key)
VALUES ('seed_pb_pii_exec', 'seed_pii_no_exec', 'containers.manage')
ON CONFLICT (id) DO NOTHING;

-- ─── 3. Read-only containers deny delete ────────────────────────────
INSERT INTO rbac_policies (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
VALUES (
    'seed_readonly_no_delete',
    'Read-only containers deny delete',
    'Refuses containers.delete when the container has mcharbor.readonly=true.',
    'deny',
    'container',
    'resource.labels["mcharbor.readonly"] == "true"',
    10,
    0,
    '__legacy_owner__'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO rbac_policy_bindings (id, policy_id, permission_key)
VALUES ('seed_pb_ro_del', 'seed_readonly_no_delete', 'containers.delete')
ON CONFLICT (id) DO NOTHING;

-- ─── 4. Team ownership gate (allow) ─────────────────────────────────
INSERT INTO rbac_policies (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
VALUES (
    'seed_team_ownership',
    'Team ownership matches user label',
    'Allows containers.manage when the container has mcharbor.team=label and the user has the corresponding team label.',
    'allow',
    'container',
    'resource.labels["mcharbor.team"] !== undefined && user.labels["team"] == resource.labels["mcharbor.team"]',
    50,
    0,
    '__legacy_owner__'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO rbac_policy_bindings (id, policy_id, permission_key)
VALUES ('seed_pb_team_manage', 'seed_team_ownership', 'containers.manage')
ON CONFLICT (id) DO NOTHING;

-- ─── 5. Business hours restriction (deny) ───────────────────────────
INSERT INTO rbac_policies (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
VALUES (
    'seed_business_hours',
    'Defer risky actions outside business hours',
    'Refuses containers.delete when the request falls outside 09:00–17:00 UTC on weekdays.',
    'deny',
    'container',
    'now.hour < 9 || now.hour >= 17 || now.weekday == 0 || now.weekday == 6',
    200,
    0,
    '__legacy_owner__'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO rbac_policy_bindings (id, policy_id, permission_key)
VALUES ('seed_pb_bh_del', 'seed_business_hours', 'containers.delete')
ON CONFLICT (id) DO NOTHING;


-- Role presets
--
-- Predefined permission sets that admins can use as a starting point
-- for new custom roles. Backups Manager, Read-Only Auditor, Container
-- Operator, Notification Recipient — common operator patterns that
-- would otherwise require hand-picking every permission key from
-- the catalog.
--
-- The presets table is read-only from the server's perspective — preset
-- definitions are server-side seed, not user-editable. Editing a permission
-- key here doesn't migrate existing roles — it just changes what the preset
-- pulls in for new role authors.
--
-- Deliberately absent from every preset:
--
--   secrets.reveal        — the only endpoint that returns a plaintext
--                           credential. It is obtained through an approved,
--                           time-bounded elevation, never by holding a role.
--                           A sanity assertion in the `permissions` check
--                           fails if it ever appears here.
--   secrets.manage_global — creating a credential every environment reads is
--                           an administrator action, and administrators are
--                           not provisioned from a preset in this install.
--
-- read_only_auditor sees a Swarm cluster and nothing else of it. Secrets and
-- configs are left out on purpose: an auditor has no need for the object
-- names, and swarm_configs.view carries the payload on inspect.
-- container_operator keeps a running cluster running: read everything a
-- workload exposes, and scale. No image changes, no node topology, no deletes.

CREATE TABLE IF NOT EXISTS role_presets (
    id           TEXT PRIMARY KEY,
    key          TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    permissions  TEXT NOT NULL DEFAULT '[]',  -- JSON array of permission keys
    added_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO role_presets (id, key, name, description, permissions)
VALUES
    ('preset_backup_manager',
     'backup_manager',
     'Backup Manager',
     'Create / run / restore container backups. Read-only on everything else.',
     '["backup_log.view","backups.download","backups.manage","backups.view","containers.view","restore.run","restore.view"]'),

    ('preset_read_only_auditor',
     'read_only_auditor',
     'Read-Only Auditor',
     'View every resource plus the full audit log. No mutations.',
     '["api_keys.view","audit.export","audit.view","containers.view","environments.view","groups.view","images.view","logs.view","networks.view","rbac.policies.view","roles.view","secrets.view","stacks.view","swarm.view","swarm_networks.view","swarm_nodes.view","swarm_services.view","swarm_stacks.view","swarm_tasks.view","volumes.view"]'),

    ('preset_container_operator',
     'container_operator',
     'Container Operator',
     'Start, stop, restart, exec into containers, tail logs. No delete.',
     '["containers.manage","containers.view","images.view","logs.view","networks.view","secrets.apply","secrets.manage","secrets.view","stacks.manage","stacks.view","swarm.view","swarm_nodes.view","swarm_services.scale","swarm_services.view","swarm_stacks.view","swarm_tasks.view","terminal.access","volumes.view"]'),

    ('preset_notification_recipient',
     'notification_recipient',
     'Notification Recipient',
     'Receive in-app notifications and read notification channels. No management.',
     '["in_app_notifications.view","communications.view"]')
ON CONFLICT (id) DO NOTHING;


-- Time-bound elevation requests. A user with a denied permission
-- can ask an admin for a temporary grant. While `approved_until > now`,
-- EffectivePermissions returns the requested permission even if the
-- user has no role grant. After expiry, the row is ignored.
--
-- Statuses: pending, approved, denied, expired. The "expired"
-- status is set by a background sweep on read, not by a separate
-- cron job — read-time check is simpler and avoids a janitor.

CREATE TABLE IF NOT EXISTS elevation_requests (
    id              TEXT PRIMARY KEY,
    requester_id    TEXT NOT NULL,
    permission_key  TEXT NOT NULL,
    environment_id  TEXT,                  -- NULL = global
    resource_type   TEXT,
    resource_id     TEXT,
    justification   TEXT NOT NULL DEFAULT '',
    requested_minutes INTEGER NOT NULL DEFAULT 60,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','denied','expired')),
    approver_id     TEXT,
    approver_note   TEXT,
    approved_until  TEXT,
    decided_at      TEXT,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    FOREIGN KEY (requester_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_key) REFERENCES permission_catalog(key) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_elevation_requests_requester ON elevation_requests(requester_id);
CREATE INDEX IF NOT EXISTS idx_elevation_requests_status ON elevation_requests(status, created_at);
CREATE INDEX IF NOT EXISTS idx_elevation_requests_perm ON elevation_requests(permission_key, environment_id);

-- Audit log of the elevation lifecycle. Mirrors rbac_policy_history
-- shape so the existing audit-trail viewer surfaces these without
-- a custom adapter.
CREATE TABLE IF NOT EXISTS elevation_history (
    id          TEXT PRIMARY KEY,
    request_id  TEXT NOT NULL,
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,         -- 'create', 'approve', 'deny', 'expire'
    details     TEXT,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_elevation_history_request ON elevation_history(request_id, created_at);


-- plugin_permissions tracks per-plugin permission declarations so
-- the installer can scope RBAC grants to a specific origin (the
-- plugin slug). Without the table, plugin boots crash at the first
-- RegisterPermissions call and uninstall fails at "step 5 (load
-- permissions)".
--
-- Columns match core/pluginruntime/loader.go accessRegistry.AddPermission
-- (origin, permission_key, description, scope) and the UNIQUE
-- constraint matches the ON CONFLICT clause there.
CREATE TABLE IF NOT EXISTS plugin_permissions (
    origin          TEXT NOT NULL,
    permission_key  TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    scope           TEXT NOT NULL DEFAULT 'global',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (origin, permission_key)
);
CREATE INDEX IF NOT EXISTS idx_plugin_permissions_origin
    ON plugin_permissions(origin);

CREATE INDEX IF NOT EXISTS idx_user_roles_environment ON user_roles(environment_id);
CREATE INDEX IF NOT EXISTS idx_group_roles_environment ON group_roles(environment_id);


-- Grant the newly-enforced permission keys to the seeded system roles.
--
-- Context: 51 mutating routes across 13 modules shipped with no permission
-- middleware. The keys existed in the catalog and the UI already gated its
-- buttons on them; only the routes never checked. Attaching the middleware
-- closes the hole, but it also means a role that was silently allowed to
-- create a workflow (because nothing asked) now needs `workflows.manage` to
-- do it.
--
-- This hands Operator and Editor the keys that match what they can
-- already do today, so closing the hole is not felt as a lost feature. Viewer
-- gets read keys only — a Viewer that could create alerts was the bug.
--
-- Admin is untouched: its permissions are '["*"]' and the wildcard already
-- resolves every key named here.
--
-- Idempotency: the UNION inside json_group_array de-duplicates, so re-running
-- cannot add a key twice, and a role an operator has edited by hand keeps
-- every key it already had. This differs from the json_insert + NOT LIKE
-- guard used by 022_email_permissions.sql, which only works when a migration
-- touches one permission category — this one touches eleven.

-- ─── Operator: manage + delete across the operational surface ─────────
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'alerts.view','alerts.manage','alerts.delete',
               'git.view','git.manage',
               'gitops.view','gitops.manage','gitops.delete','gitops.approve',
               'notifications.view','notifications.manage',
               'reconciler.view','reconciler.manage',
               'registry.view','registry.manage',
               'schedules.view','schedules.manage','schedules.delete',
               'updates.view','updates.manage',
               'webhooks.view','webhooks.manage','webhooks.delete',
               'workflows.view','workflows.manage','workflows.delete','workflows.run',
               'custom_nodes.view','custom_nodes.manage',
               'activity.view'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE name = 'Operator' AND is_system = 1;

-- ─── Editor: manage, no delete ────────────────────────────────────────
-- Matches the seeded description: "View and manage resources without delete
-- or admin access". gitops.approve is withheld deliberately — signing off a
-- promotion gate is an approval authority, not an editing one.
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'alerts.view','alerts.manage',
               'git.view','git.manage',
               'gitops.view','gitops.manage',
               'notifications.view','notifications.manage',
               'reconciler.view','reconciler.manage',
               'registry.view','registry.manage',
               'schedules.view','schedules.manage',
               'updates.view','updates.manage',
               'webhooks.view','webhooks.manage',
               'workflows.view','workflows.manage','workflows.run',
               'custom_nodes.view','custom_nodes.manage',
               'activity.view'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE name = 'Editor' AND is_system = 1;

-- ─── Viewer: reads only ───────────────────────────────────────────────
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'alerts.view','git.view','gitops.view','notifications.view',
               'reconciler.view','registry.view','schedules.view',
               'updates.view','webhooks.view','workflows.view',
               'custom_nodes.view','activity.view'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE name = 'Viewer' AND is_system = 1;

-- ─── Deliberately NOT granted: environments.manage ────────────────────
--
-- The agent endpoints (/agents/{envId}/regenerate-token, /deploy,
-- /install-token, and /agents/direct-transfer-test) require
-- `environments.manage`. No seeded role but Admin holds it, and this
-- file does not change that.
--
-- An agent install token is a bearer credential that yields Docker socket
-- control over the target host. Issuing one is an administrative act.
-- Operators who genuinely need to deploy agents should be granted
-- `environments.manage` explicitly, as a decision an admin makes — not as a
-- side effect of a migration.


-- Grant the Swarm permission set to the seeded system roles.
--
-- role_admin is untouched: its permissions are '["*"]' and the wildcard
-- already resolves every swarm.* key.
--
-- `swarm.manage` — init, join, leave — reaches no seeded role but the wildcard
-- admin, because reconfiguring cluster membership is not a routine operator
-- action.

-- --- Reads: viewer, editor, operator --------------------------------------
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'swarm.view','swarm_services.view','swarm_tasks.view',
               'swarm_nodes.view','swarm_secrets.view','swarm_configs.view',
               'swarm_stacks.view','swarm_networks.view'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id IN ('role_viewer','role_editor','role_operator');

-- --- Non-destructive writes: editor, operator -----------------------------
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'swarm_services.manage','swarm_services.scale',
               'swarm_nodes.manage','swarm_secrets.manage',
               'swarm_configs.manage','swarm_stacks.manage'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id IN ('role_editor','role_operator');

-- --- Deletes: operator only -----------------------------------------------
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'swarm_services.delete','swarm_nodes.delete',
               'swarm_secrets.delete','swarm_configs.delete',
               'swarm_stacks.delete'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id = 'role_operator';


-- Rename container_backups.* to backups.* / restore.*.
--
-- The old keys were declared but dead: they existed as constants, catalog
-- entries and frontend PERM keys, yet every container_backups route actually
-- gated on containers.view / containers.manage and no frontend call site
-- referenced them. A role granted "container_backups.manage" therefore got
-- nothing, while anyone with "containers.manage" could run and delete backups.
--
-- The new keys are wired to the routes for real, so existing grants are
-- carried across or roles would silently lose backup access:
--
--   container_backups.view    -> backups.view
--   container_backups.manage  -> backups.manage + backups.download
--   container_backups.delete  -> backups.delete
--   container_backups.restore -> restore.view + restore.run
--
-- manage gains download, and restore gains its browse half, so that no role
-- ends up able to start a restore without being able to see what it is about
-- to write.
--
-- Admin roles hold '["*"]' and are untouched — the wildcard already resolves
-- every key.
--
-- Idempotency: UNION de-duplicates, and the final filter drops the old keys, so
-- re-running is a no-op.

-- ─── Roles: add the replacement keys ──────────────────────────────────
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT 'backups.view'
             WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                            WHERE value = 'container_backups.view')
           UNION
           SELECT 'backups.manage'
             WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                            WHERE value = 'container_backups.manage')
           UNION
           SELECT 'backups.download'
             WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                            WHERE value = 'container_backups.manage')
           UNION
           SELECT 'backups.delete'
             WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                            WHERE value = 'container_backups.delete')
           UNION
           SELECT 'restore.view'
             WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                            WHERE value = 'container_backups.restore')
           UNION
           SELECT 'restore.run'
             WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                            WHERE value = 'container_backups.restore')
       )
   )
 WHERE EXISTS (
       SELECT 1 FROM json_each(roles.permissions)
        WHERE value LIKE 'container_backups.%'
   );

-- ─── Roles: drop the retired keys ─────────────────────────────────────
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM json_each(roles.permissions)
        WHERE value NOT LIKE 'container_backups.%'
   )
 WHERE EXISTS (
       SELECT 1 FROM json_each(roles.permissions)
        WHERE value LIKE 'container_backups.%'
   );


-- api_keys.view: listing keys is separate from managing them.
-- Grant it to the Viewer system role, to every role that already carries
-- audit.view (the auditor shape), and to every role that already carries
-- api_keys.manage so nobody who could list keys yesterday loses the list
-- today. Wildcard roles are skipped by construction.
UPDATE roles
   SET permissions = json_insert(permissions, '$[#]', 'api_keys.view'),
       updated_at  = CURRENT_TIMESTAMP
 WHERE (
         (name = 'Viewer' AND is_system = 1)
         OR EXISTS (SELECT 1 FROM json_each(roles.permissions)
                     WHERE value IN ('audit.view', 'api_keys.manage'))
       )
   AND NOT EXISTS (SELECT 1 FROM json_each(roles.permissions)
                    WHERE value IN ('api_keys.view', '*'));


-- Give the built-in roles the reporting and kubernetes add-on keys.
--
-- Fresh installs receive the same grants from the plugin manifest at install
-- time (`presets:` on each `adds_permissions` entry); this is for installs
-- that already have the plugins in place. On an install without the plugins
-- the grants name keys no catalog defines, which is inert and becomes live
-- the moment the plugin is installed.
--
-- Tiering follows the host's own convention: viewer reads, editor also
-- creates and changes, operator also deletes. Global cluster-level keys
-- (kubernetes.clusters.manage, swarm.manage) stay admin-only.

-- --- Reads: viewer, editor, operator --------------------------------------
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'reporting.read',
               'kubernetes.view'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id IN ('role_viewer','role_editor','role_operator');

-- --- Non-destructive writes: editor, operator -----------------------------
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'reporting.generate','reporting.schedule',
               'kubernetes.logs','kubernetes.manage'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id IN ('role_editor','role_operator');

-- --- Deletes and management: operator only --------------------------------
UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'reporting.manage',
               'kubernetes.delete'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id = 'role_operator';


-- Permission catalog hygiene (sanity: permissions checks 7 and 11).
--
-- Grants. Three view keys existed in the catalog but were enforced on no
-- route, so the seeded roles were never given them. Their routes are gated
-- (dashboard/routes.go, events/routes.go, in_app_notifications/routes.go);
-- without these grants every non-admin would lose the dashboard, the Docker
-- event stream and the notification bell.

UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM (
           SELECT value FROM json_each(roles.permissions)
           UNION
           SELECT value FROM json_each(json_array(
               'dashboard.view',
               'events.view',
               'in_app_notifications.view'
           ))
       )
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE id IN ('role_viewer','role_editor','role_operator');

-- Prunes. Migration 022 granted email_servers.view / manage to the seeded
-- roles for a module that has since moved into the email plugins; no host
-- const or plugin manifest declares the key any more.

UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM json_each(roles.permissions)
        WHERE value NOT IN ('email_servers.view', 'email_servers.manage')
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                WHERE value IN ('email_servers.view', 'email_servers.manage'));

-- Retire the legacy `modules/notifications` keys. Channel management lives in
-- `modules/communications`, gated by `communications.view` /
-- `communications.manage`. Nothing else enforced `notifications.view` /
-- `notifications.manage`.
--
-- `in_app_notifications.view` is a different key for a different module (the
-- notification bell) and is deliberately untouched.

UPDATE roles
   SET permissions = (
       SELECT json_group_array(value) FROM json_each(roles.permissions)
        WHERE value NOT IN ('notifications.view', 'notifications.manage')
   ),
   updated_at = CURRENT_TIMESTAMP
 WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions)
                WHERE value IN ('notifications.view', 'notifications.manage'));
