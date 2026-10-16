-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- One rebuild to the final identity-provider shape, and SCIM 2.0 endpoints,
-- sync log and user mappings.

-- The 2.0.8 columns are, verified against a database built from the committed
-- migrations alone:
--   id name provider_type enabled client_id client_secret tenant_id domain
--   issuer_url metadata_url entity_id sp_certificate sp_private_key scopes
--   auto_provision default_role_id group_mapping_enabled group_mappings
--   auto_import_groups created_at updated_at
--
-- `config`, the ten ldap_* columns and client_secret_secret_id did not exist
-- there, so they take their defaults rather than being selected.
--
-- Why a rebuild rather than ALTERs: provider_type carried
-- CHECK(provider_type IN (...)) and every plugin-owned provider type had to be
-- added to that list. SQLite cannot ALTER a constraint away, and dropping it is
-- the point — after this there is nothing left to widen.

PRAGMA foreign_keys = OFF;

CREATE TABLE identity_providers_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    tenant_id TEXT,
    domain TEXT,
    issuer_url TEXT,
    metadata_url TEXT,
    entity_id TEXT,
    sp_certificate TEXT,
    sp_private_key TEXT,
    scopes TEXT NOT NULL DEFAULT 'openid profile email',
    auto_provision INTEGER NOT NULL DEFAULT 1,
    default_role_id TEXT REFERENCES roles(id) ON DELETE SET NULL,
    group_mapping_enabled INTEGER NOT NULL DEFAULT 0,
    group_mappings TEXT NOT NULL DEFAULT '[]',
    auto_import_groups INTEGER NOT NULL DEFAULT 0,
    config TEXT NOT NULL DEFAULT '{}',
    ldap_url TEXT,
    ldap_bind_dn TEXT,
    ldap_bind_password_encrypted TEXT,
    ldap_user_base_dn TEXT,
    ldap_user_filter TEXT,
    ldap_user_attr TEXT DEFAULT 'uid',
    ldap_email_attr TEXT DEFAULT 'mail',
    ldap_name_attr TEXT DEFAULT 'cn',
    ldap_start_tls INTEGER NOT NULL DEFAULT 1,
    ldap_skip_tls_verify INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    client_secret_secret_id TEXT REFERENCES secrets(id)
);

INSERT INTO identity_providers_new
    (id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain,
     issuer_url, metadata_url, entity_id, sp_certificate, sp_private_key, scopes,
     auto_provision, default_role_id, group_mapping_enabled, group_mappings,
     auto_import_groups, created_at, updated_at)
SELECT
     id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain,
     issuer_url, metadata_url, entity_id, sp_certificate, sp_private_key, scopes,
     auto_provision, default_role_id, group_mapping_enabled, group_mappings,
     auto_import_groups, created_at, updated_at
FROM identity_providers;

DROP TABLE identity_providers;
ALTER TABLE identity_providers_new RENAME TO identity_providers;

CREATE INDEX idx_identity_providers_type ON identity_providers(provider_type);
CREATE INDEX idx_identity_providers_enabled ON identity_providers(enabled);
CREATE INDEX IF NOT EXISTS idx_identity_providers_default_role ON identity_providers(default_role_id);
CREATE INDEX IF NOT EXISTS idx_oidc_states_provider ON oidc_states(provider_id);

PRAGMA foreign_keys = ON;


-- SCIM 2.0 endpoint registry
--
-- Each row represents one configured SCIM client. The bearer token
-- is encrypted at rest (AES-256-GCM via core/encryption). A stable
-- keyed hash is stored alongside for O(1) lookup during inbound
-- bearer-token auth on /api/scim/v2/... routes.
--
-- base_url points at the SCIM 2.0 server we will pull from (the
-- external IdP). McHarbor itself does not push — that is by
-- product decision (pull-only mode).
--
-- poll_schedule is a 5-field cron expression (default `*/15 * * * *`).
-- last_sync_* columns are updated by the poller so admins can see
-- when each endpoint was last contacted and whether it succeeded.
--
-- next_sync_at lets admins trigger an immediate pull via the API without
-- restarting the poller. The request is stored as a wall-clock timestamp
-- the poller checks on each tick, and the column doubles as the admin's
-- "last manual sync" record so they can see whether the request landed.

CREATE TABLE IF NOT EXISTS scim_endpoints (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL UNIQUE,
    base_url             TEXT NOT NULL,
    auth_type            TEXT NOT NULL DEFAULT 'bearer'
                              CHECK (auth_type IN ('bearer', 'basic', 'none')),
    encrypted_token      TEXT NOT NULL DEFAULT '',
    token_hash           TEXT NOT NULL DEFAULT '',
    username             TEXT NOT NULL DEFAULT '',
    poll_schedule        TEXT NOT NULL DEFAULT '*/15 * * * *',
    enabled              INTEGER NOT NULL DEFAULT 1,
    last_sync_at         TEXT,
    last_sync_status     TEXT NOT NULL DEFAULT 'pending'
                              CHECK (last_sync_status IN ('pending','success','failed')),
    last_sync_error      TEXT NOT NULL DEFAULT '',
    users_count          INTEGER NOT NULL DEFAULT 0,
    groups_count         INTEGER NOT NULL DEFAULT 0,
    created_by           TEXT,
    created_at           TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at           TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    next_sync_at         TEXT,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_scim_endpoints_enabled
    ON scim_endpoints(enabled);
CREATE INDEX IF NOT EXISTS idx_scim_endpoints_token_hash
    ON scim_endpoints(token_hash);
CREATE INDEX IF NOT EXISTS idx_scim_endpoints_created_by ON scim_endpoints(created_by);


-- SCIM sync history and user mappings
--
-- scim_sync_log captures every sync run with timing + counts so admins
-- can see when each endpoint last succeeded. Old rows are pruned by
-- the audit-retention settings (default 90 days).
--
-- scim_user_mappings records which external SCIM user id maps to which
-- McHarbor user id. We do NOT auto-create McHarbor users on first
-- sight — admins opt-in per endpoint by configuring an `auto_provision`
-- flag in v2; for now mappings are populated only when the user
-- already exists. Unmatched rows stay in scim_user_mappings with a
-- NULL mcharbor_user_id so admins can review and resolve manually.

CREATE TABLE IF NOT EXISTS scim_sync_log (
    id              TEXT PRIMARY KEY,
    endpoint_id     TEXT NOT NULL,
    direction       TEXT NOT NULL CHECK (direction IN ('pull','push')),
    started_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    finished_at     TEXT,
    status          TEXT NOT NULL CHECK (status IN ('success','failed','running')),
    users_created   INTEGER NOT NULL DEFAULT 0,
    users_updated   INTEGER NOT NULL DEFAULT 0,
    users_deleted   INTEGER NOT NULL DEFAULT 0,
    groups_created  INTEGER NOT NULL DEFAULT 0,
    groups_updated  INTEGER NOT NULL DEFAULT 0,
    groups_deleted  INTEGER NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (endpoint_id) REFERENCES scim_endpoints(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scim_sync_log_endpoint
    ON scim_sync_log(endpoint_id, started_at);
CREATE INDEX IF NOT EXISTS idx_scim_sync_log_status
    ON scim_sync_log(status, started_at);

CREATE TABLE IF NOT EXISTS scim_user_mappings (
    id                  TEXT PRIMARY KEY,
    endpoint_id         TEXT NOT NULL,
    scim_user_id        TEXT NOT NULL,
    mcharbor_user_id    TEXT,
    scim_username       TEXT NOT NULL DEFAULT '',
    scim_email          TEXT NOT NULL DEFAULT '',
    last_seen_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (endpoint_id, scim_user_id),
    FOREIGN KEY (endpoint_id) REFERENCES scim_endpoints(id) ON DELETE CASCADE,
    FOREIGN KEY (mcharbor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_scim_user_mappings_endpoint
    ON scim_user_mappings(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_scim_user_mappings_user
    ON scim_user_mappings(mcharbor_user_id);
