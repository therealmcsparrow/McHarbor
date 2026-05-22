-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE identity_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK(provider_type IN ('entra_id', 'google')),
    enabled INTEGER NOT NULL DEFAULT 0,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    tenant_id TEXT,
    domain TEXT,
    scopes TEXT NOT NULL DEFAULT 'openid profile email',
    auto_provision INTEGER NOT NULL DEFAULT 1,
    default_role_id TEXT REFERENCES roles(id) ON DELETE SET NULL,
    group_mapping_enabled INTEGER NOT NULL DEFAULT 0,
    group_mappings TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_identity_providers_type ON identity_providers(provider_type);
CREATE INDEX idx_identity_providers_enabled ON identity_providers(enabled);

ALTER TABLE users ADD COLUMN identity_provider_id TEXT REFERENCES identity_providers(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN external_id TEXT;
CREATE INDEX idx_users_external_id ON users(external_id);
CREATE INDEX idx_users_identity_provider ON users(identity_provider_id);

CREATE TABLE oidc_states (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    state TEXT NOT NULL UNIQUE,
    nonce TEXT NOT NULL,
    redirect_url TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_oidc_states_state ON oidc_states(state);
CREATE INDEX idx_oidc_states_expires ON oidc_states(expires_at);
