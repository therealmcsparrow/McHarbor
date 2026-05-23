-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

PRAGMA foreign_keys = OFF;

CREATE TABLE identity_providers_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK(provider_type IN ('entra_id', 'google', 'generic_oidc')),
    enabled INTEGER NOT NULL DEFAULT 0,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    tenant_id TEXT,
    domain TEXT,
    issuer_url TEXT,
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
    issuer_url, scopes, auto_provision, default_role_id, group_mapping_enabled,
    group_mappings, auto_import_groups, created_at, updated_at
)
SELECT
    id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain,
    NULL, scopes, auto_provision, default_role_id, group_mapping_enabled,
    group_mappings, auto_import_groups, created_at, updated_at
FROM identity_providers;

DROP TABLE identity_providers;
ALTER TABLE identity_providers_new RENAME TO identity_providers;

CREATE INDEX idx_identity_providers_type ON identity_providers(provider_type);
CREATE INDEX idx_identity_providers_enabled ON identity_providers(enabled);

PRAGMA foreign_keys = ON;
