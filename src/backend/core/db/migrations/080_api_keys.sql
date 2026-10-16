-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- API keys governed by roles and permissions.
-- Spec: docs/superpowers/specs/2026-09-02-api-key-rbac-design.md
--
-- A key gets its own authorization identity: assigned roles, explicit
-- permission grants (the existing api_keys.scopes JSON column), and an
-- optional environment fence. The RBAC service intersects that identity
-- with the owner's permissions on every request, so a key never exceeds
-- the person who holds it.

CREATE TABLE IF NOT EXISTS api_key_roles (
    id          TEXT PRIMARY KEY,
    api_key_id  TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    role_id     TEXT NOT NULL REFERENCES roles(id)    ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE(api_key_id, role_id)
);
CREATE INDEX IF NOT EXISTS idx_api_key_roles_key  ON api_key_roles(api_key_id);
CREATE INDEX IF NOT EXISTS idx_api_key_roles_role ON api_key_roles(role_id);

CREATE TABLE IF NOT EXISTS api_key_environments (
    id              TEXT PRIMARY KEY,
    api_key_id      TEXT NOT NULL REFERENCES api_keys(id)     ON DELETE CASCADE,
    environment_id  TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE(api_key_id, environment_id)
);
CREATE INDEX IF NOT EXISTS idx_api_key_environments_key ON api_key_environments(api_key_id);
CREATE INDEX IF NOT EXISTS idx_api_key_environments_env ON api_key_environments(environment_id);

-- Backfill: every existing key inherits every role its owner holds today,
-- through direct assignments and through group membership, global and
-- environment-scoped alike. No environment rows are written, so keys stay
-- unfenced. The intersection rule in core/rbac keeps this safe: the
-- owner's own scoping still applies at evaluation time.
--
-- Ids are deterministic composites so the statement is idempotent under
-- INSERT OR IGNORE and re-running the migration cannot duplicate rows.
INSERT OR IGNORE INTO api_key_roles (id, api_key_id, role_id, created_at, updated_at)
SELECT ak.id || ':' || ur.role_id, ak.id, ur.role_id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
  FROM api_keys ak
  INNER JOIN user_roles ur ON ur.user_id = ak.user_id;

INSERT OR IGNORE INTO api_key_roles (id, api_key_id, role_id, created_at, updated_at)
SELECT ak.id || ':' || gr.role_id, ak.id, gr.role_id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
  FROM api_keys ak
  INNER JOIN group_members gm ON gm.user_id = ak.user_id
  INNER JOIN group_roles gr   ON gr.group_id = gm.group_id;

-- Audit vocabulary for the new update route and the delete branch of the
-- existing DELETE route. A grant change or a permanent delete is exactly
-- what an auditor wants flagged, hence warning.
INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES ('audit_action_083', 'api_keys', 'api_key.updated', 'warning',
        'API key roles, permissions, or environments were changed')
ON CONFLICT(action) DO NOTHING;

INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES ('audit_action_084', 'api_keys', 'api_key.deleted', 'warning',
        'Revoked API key was permanently deleted')
ON CONFLICT(action) DO NOTHING;

INSERT OR IGNORE INTO audit_action_route_map
    (id, module, method, path, handler, action, entity_type, entity_id_source, audit_required, rationale)
VALUES ('audit_route_022', 'api_keys', 'PATCH', '/api-keys/{id}', 'HandleUpdate',
        'api_key.updated', 'api_key', 'path.id', 1, 'Credential grant change');


-- Per-API-key request telemetry, deleted-key tombstones, and their retention:
-- what has this key actually been doing, and what did a key that no longer
-- exists used to be allowed to do.

-- ─── Usage events ─────────────────────────────────────────────────────
--
-- api_key_id is a logical reference with deliberately NO foreign key.
--
-- A cascade here would destroy the history at exactly the moment it matters
-- most: the key was deleted, and the question "what did it do before it went
-- away" is the whole point of keeping the rows. Retention owns the lifetime
-- of this table instead, and orphaned rows stay queryable through the
-- tombstone that carries the same id.
--
-- Every column is metadata. There is no token, no hash, no Authorization
-- header, no cookie, no request or response body, and no query string here,
-- and nothing may add one: route_pattern stores a chi template such as
-- /containers/{id} precisely so raw URLs never land in this table.
CREATE TABLE IF NOT EXISTS api_key_usage_events (
    id                TEXT PRIMARY KEY,
    api_key_id        TEXT NOT NULL,
    occurred_at       TEXT NOT NULL,
    method            TEXT NOT NULL,
    route_pattern     TEXT NOT NULL,
    status_code       INTEGER NOT NULL,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    -- accepted | revoked | expired | owner_invalid
    auth_outcome      TEXT NOT NULL,
    environment_id    TEXT,
    peer_ip           TEXT NOT NULL DEFAULT '',
    client_ip         TEXT NOT NULL DEFAULT '',
    client_ip_source  TEXT NOT NULL DEFAULT 'peer',
    user_agent        TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

-- The detail page's default read: one key, newest first.
CREATE INDEX IF NOT EXISTS idx_api_key_usage_key_time
    ON api_key_usage_events(api_key_id, occurred_at DESC);
-- Retention sweeps by age across every key.
CREATE INDEX IF NOT EXISTS idx_api_key_usage_occurred
    ON api_key_usage_events(occurred_at);
-- The status filter on the drill-down table.
CREATE INDEX IF NOT EXISTS idx_api_key_usage_key_status_time
    ON api_key_usage_events(api_key_id, status_code, occurred_at DESC);
-- The endpoint filter on the drill-down table.
CREATE INDEX IF NOT EXISTS idx_api_key_usage_key_route_time
    ON api_key_usage_events(api_key_id, route_pattern, occurred_at DESC);

-- ─── Deleted-key tombstones ───────────────────────────────────────────
--
-- Deleting a revoked key destroys its credential: the api_keys row goes, and
-- with it key_hash and the grant joins. What survives is this row — safe
-- metadata plus JSON snapshots of what the key was allowed to do at the
-- moment it was deleted.
--
-- The snapshots are frozen on purpose. Joining a tombstone back to live role
-- rows would answer "what would these role names mean today", which is not
-- the question anyone asks about a deleted credential, and would show nothing
-- at all once a role is itself deleted.
--
-- There is deliberately no key_hash column. Nothing in this table can
-- authenticate a request.
CREATE TABLE IF NOT EXISTS api_key_tombstones (
    id                        TEXT PRIMARY KEY,
    user_id                   TEXT,
    username                  TEXT NOT NULL DEFAULT '',
    name                      TEXT NOT NULL,
    key_prefix                TEXT NOT NULL,
    roles_snapshot            TEXT NOT NULL DEFAULT '[]',
    explicit_scopes_snapshot  TEXT NOT NULL DEFAULT '[]',
    configured_perms_snapshot TEXT NOT NULL DEFAULT '[]',
    effective_perms_snapshot  TEXT NOT NULL DEFAULT '[]',
    environments_snapshot     TEXT NOT NULL DEFAULT '[]',
    expires_at                TEXT,
    last_used_at              TEXT,
    key_created_at            TEXT NOT NULL,
    key_updated_at            TEXT NOT NULL,
    deleted_at                TEXT NOT NULL,
    created_at                TEXT NOT NULL,
    updated_at                TEXT NOT NULL
);

-- Retention sweeps tombstones by deletion age, not by request age.
CREATE INDEX IF NOT EXISTS idx_api_key_tombstones_deleted
    ON api_key_tombstones(deleted_at);
-- The list merges tombstones per owner alongside live keys.
CREATE INDEX IF NOT EXISTS idx_api_key_tombstones_user
    ON api_key_tombstones(user_id);

-- ─── Credential-destruction safety net ────────────────────────────────
--
-- The Go service archives a key before deleting it, on both routes that
-- delete one: the explicit second DELETE, and the owner-deletion path. This
-- trigger is the belt to those braces.
--
-- api_keys.user_id carries ON DELETE CASCADE, so deleting a user destroys
-- their keys inside SQLite with no Go code in the loop. If any path ever
-- reaches that cascade without going through the service — a future feature,
-- a manual repair, a migration — the credential would vanish leaving no
-- record that it ever existed. This makes that impossible.
--
-- WHEN NOT EXISTS means the service's richer snapshot wins whenever it ran
-- first; the trigger only writes for a key nothing else archived. Such a row
-- carries an empty effective_perms_snapshot, because the owner intersection
-- needs the RBAC engine and cannot be computed in SQL. That is reported
-- honestly rather than guessed at.
--
-- OLD.key_hash is deliberately not referenced. Nothing in the tombstone can
-- authenticate anything.
CREATE TRIGGER IF NOT EXISTS trg_api_keys_archive_before_delete
BEFORE DELETE ON api_keys
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM api_key_tombstones WHERE id = OLD.id)
BEGIN
    INSERT INTO api_key_tombstones (
        id, user_id, username, name, key_prefix,
        roles_snapshot, explicit_scopes_snapshot, configured_perms_snapshot,
        effective_perms_snapshot, environments_snapshot,
        expires_at, last_used_at, key_created_at, key_updated_at,
        deleted_at, created_at, updated_at
    ) VALUES (
        OLD.id,
        OLD.user_id,
        COALESCE((SELECT u.username FROM users u WHERE u.id = OLD.user_id), ''),
        OLD.name,
        OLD.key_prefix,
        COALESCE((
            SELECT json_group_array(json_object(
                'id', r.id,
                'name', r.name,
                'description', COALESCE(r.description, ''),
                'permissions', json(r.permissions)
            ))
            FROM api_key_roles akr
            JOIN roles r ON r.id = akr.role_id
            WHERE akr.api_key_id = OLD.id
        ), '[]'),
        COALESCE(OLD.scopes, '[]'),
        COALESCE((
            SELECT json_group_array(value) FROM (
                SELECT je.value AS value
                  FROM api_key_roles akr
                  JOIN roles r ON r.id = akr.role_id
                  JOIN json_each(r.permissions) je
                 WHERE akr.api_key_id = OLD.id
                UNION
                SELECT js.value FROM json_each(COALESCE(OLD.scopes, '[]')) js
            )
        ), '[]'),
        '[]',
        COALESCE((
            SELECT json_group_array(json_object('id', e.id, 'name', e.name))
            FROM api_key_environments ake
            JOIN environments e ON e.id = ake.environment_id
            WHERE ake.api_key_id = OLD.id
        ), '[]'),
        OLD.expires_at,
        OLD.last_used_at,
        OLD.created_at,
        OLD.updated_at,
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    );
END;

-- ─── Retention setting ────────────────────────────────────────────────
--
-- Seeded explicitly so an operator opening Settings sees 90 rather than a
-- blank that only the Go default knows about. 0 means keep forever, the same
-- convention the other log retentions use.
--
-- Events age from occurred_at; tombstones age from deleted_at. Those are
-- different clocks on purpose: a key deleted today keeps its full history
-- window, and a key that has been quiet for a year does not resurrect one.
INSERT INTO settings (id, key, value, category, created_at, updated_at)
VALUES ('setting_retention_api_key_usage_days', 'retention_api_key_usage_days', '90', 'retention', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO NOTHING;

-- ─── Audit vocabulary ─────────────────────────────────────────────────
--
-- The scheduled pruner does not write one audit row per deleted event; it
-- logs a count. Only the operator-initiated clear is audited, and it is a
-- warning because it destroys evidence on purpose.
INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES ('audit_action_085', 'api_keys', 'api_key.usage_cleared', 'warning',
        'API key usage history and deleted-key tombstones were cleared manually')
ON CONFLICT(action) DO NOTHING;

INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES ('audit_action_086', 'api_keys', 'api_key.archived', 'warning',
        'API keys were archived to tombstones because their owner was deleted')
ON CONFLICT(action) DO NOTHING;

INSERT OR IGNORE INTO audit_action_route_map
    (id, module, method, path, handler, action, entity_type, entity_id_source, audit_required, rationale)
VALUES ('audit_route_023', 'api_keys', 'DELETE', '/api-keys/usage', 'HandleClearUsage',
        'api_key.usage_cleared', 'api_key_usage', 'none', 1, 'Destroys access telemetry');
