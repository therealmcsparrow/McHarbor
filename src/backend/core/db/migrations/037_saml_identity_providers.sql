-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

PRAGMA foreign_keys = OFF;

CREATE TABLE identity_providers_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK(provider_type IN ('entra_id', 'google', 'generic_oidc', 'saml_2_0')),
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
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO identity_providers_new (
    id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain,
    issuer_url, metadata_url, entity_id, sp_certificate, sp_private_key, scopes,
    auto_provision, default_role_id, group_mapping_enabled, group_mappings,
    auto_import_groups, created_at, updated_at
)
SELECT
    id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain,
    issuer_url, NULL, NULL, NULL, NULL, scopes, auto_provision, default_role_id,
    group_mapping_enabled, group_mappings, auto_import_groups, created_at, updated_at
FROM identity_providers;

DROP TABLE identity_providers;
ALTER TABLE identity_providers_new RENAME TO identity_providers;

CREATE INDEX idx_identity_providers_type ON identity_providers(provider_type);
CREATE INDEX idx_identity_providers_enabled ON identity_providers(enabled);

CREATE TABLE IF NOT EXISTS saml_requests (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL UNIQUE,
    relay_state TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_saml_requests_provider ON saml_requests(provider_id);
CREATE INDEX idx_saml_requests_relay_state ON saml_requests(relay_state);
CREATE INDEX idx_saml_requests_expires ON saml_requests(expires_at);

PRAGMA foreign_keys = ON;
