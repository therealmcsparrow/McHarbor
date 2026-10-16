-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- The secrets vault. One table for the current value, one for the rotation
-- history, one for "who is using this".
--
-- Design notes that are not obvious from the DDL:
--
--  * `scope_key` and `ref_key` are SHA-256 hex digests computed in Go on every
--    write. Scope uniqueness is "one NAME per (environment, stack)", which on
--    SQLite and Postgres could be a unique index over COALESCE(...). MariaDB
--    has no functional key parts, so it needs a real column. Hex keeps the
--    length fixed at 64, well inside the VARCHAR(255) that MySQL requires for
--    a keyed column.
--
--  * `value` is deliberately NOT part of any key, so it stays TEXT on MySQL
--    and can hold a multi-KB kubeconfig or TLS certificate. Do not add an
--    index to it.
--
--  * `kind` is presentation and validation only — every kind is stored,
--    encrypted and resolved identically — so it carries no CHECK. The shape
--    is enforced in Go: NormalizeKind lowercases and validateKind requires
--    ^[a-z][a-z0-9_]{0,31}$.
--
--  * `scope` keeps its CHECK. That set is genuinely closed — the resolver
--    walks stack -> environment -> global -> application — and a value
--    nobody taught `lookup` about would be a silent resolution hole rather
--    than a label. `application` is an instance-wide credential that belongs
--    to McHarbor itself rather than to a workload; it resolves AFTER
--    `global`, and scopeKeyFor() in service.go adds a discriminator for it
--    so it cannot collide with a global secret on scope_key. See
--    core/secrets/resolver.go.
--
--  * `group_name` is an optional, free-text grouping. It is presentation
--    only: no resolver, no scope_key, no uniqueness. It is `group_name`, not
--    `group`, because GROUP is a reserved word in every SQL dialect this
--    ships against.

CREATE TABLE IF NOT EXISTS secrets (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT,
    kind             TEXT NOT NULL DEFAULT 'env',
    group_name       TEXT,
    scope            TEXT NOT NULL DEFAULT 'global'
                     CHECK(scope IN ('global','environment','stack','application')),
    environment_id   TEXT REFERENCES environments(id) ON DELETE CASCADE,
    stack_name       TEXT,
    scope_key        TEXT NOT NULL,
    value            TEXT NOT NULL,
    key_id           TEXT NOT NULL DEFAULT 'default',
    version          INTEGER NOT NULL DEFAULT 1,
    generated        INTEGER NOT NULL DEFAULT 0,
    system           INTEGER NOT NULL DEFAULT 0,
    origin_module    TEXT,
    last_used_at     TEXT,
    last_revealed_at TEXT,
    created_by       TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    auto_apply       INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_scope_key ON secrets(scope_key);
CREATE INDEX IF NOT EXISTS idx_secrets_env        ON secrets(environment_id);
CREATE INDEX IF NOT EXISTS idx_secrets_scope      ON secrets(scope);
CREATE INDEX IF NOT EXISTS idx_secrets_origin     ON secrets(origin_module);
CREATE INDEX IF NOT EXISTS idx_secrets_name       ON secrets(name);
CREATE INDEX IF NOT EXISTS idx_secrets_group      ON secrets(group_name);
CREATE INDEX IF NOT EXISTS idx_secrets_created_by ON secrets(created_by);

CREATE TABLE IF NOT EXISTS secret_versions (
    id          TEXT PRIMARY KEY,
    secret_id   TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
    version     INTEGER NOT NULL,
    value       TEXT NOT NULL,
    rotated_by  TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_secret_versions_secret
    ON secret_versions(secret_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_secret_versions_rotated_by ON secret_versions(rotated_by);

CREATE TABLE IF NOT EXISTS secret_references (
    id              TEXT PRIMARY KEY,
    secret_id       TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
    resource_type   TEXT NOT NULL,
    resource_id     TEXT NOT NULL,
    environment_id  TEXT,
    field           TEXT,
    ref_key         TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    applied_version INTEGER NOT NULL DEFAULT 0,
    resource_name   TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secret_refs_unique ON secret_references(ref_key);
CREATE INDEX IF NOT EXISTS idx_secret_refs_resource
    ON secret_references(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_secret_refs_secret ON secret_references(secret_id);
CREATE INDEX IF NOT EXISTS idx_secret_refs_applied
    ON secret_references(secret_id, applied_version);


-- Every operator-typed credential the application stores gains a pointer
-- into the vault, beside the column that already holds it.
--
-- Both columns coexist. The Go backfill (modules/secrets/adopt.go, invoked at
-- start-up) fills the `_secret_id` side; every module reads through the vault
-- when the id is set and falls back to its own column when it is not, so a
-- half-migrated database works. **The legacy columns are not dropped here and
-- must not be dropped in the same release** — a dropped column is
-- unrecoverable if the backfill had a bug.
--
-- Deliberately NOT here, and the omissions are security decisions rather than
-- scope cuts:
--
--   * `user_mfa` TOTP seeds and recovery codes. A vault reveal would let an
--     administrator read a user's second factor and impersonate their MFA
--     indefinitely, without the user ever knowing. Settled 2026-09-13; a Go
--     test asserts that no `secrets` row ever carries origin_module = 'mfa'.
--   * Machine-rotated OAuth tokens: `storage_locations.access_token` /
--     `.refresh_token` / `.token`, `identity_providers` rotating tokens, and
--     the `ai_provider_connections.oauth_access_token` / `_refresh_token`
--     pair. These refresh hourly; versioning them would write secret_versions
--     rows continuously for values no human needs to see or roll back.
--   * `licenses`. A licence key grants access to nothing of the user's.

ALTER TABLE webhooks ADD COLUMN secret_secret_id TEXT REFERENCES secrets(id);

ALTER TABLE registries ADD COLUMN password_secret_id TEXT REFERENCES secrets(id);

ALTER TABLE git_repos ADD COLUMN credential_secret_id TEXT REFERENCES secrets(id);

ALTER TABLE workflows ADD COLUMN variables_secret_id TEXT REFERENCES secrets(id);

-- Storage locations: the nine fields an operator types. access_token,
-- refresh_token and token are excluded — the OAuth consent flow rewrites those
-- columns on its own schedule.
ALTER TABLE storage_locations ADD COLUMN password_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN private_key_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN passphrase_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN ca_certificate_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN client_certificate_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN client_key_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN access_key_id_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN secret_access_key_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE storage_locations ADD COLUMN client_secret_secret_id TEXT REFERENCES secrets(id);

-- settings is a key/value table, so the pointer column is only meaningful on
-- the two TLS rows. The backfill narrows to them with a WHERE clause.
ALTER TABLE settings ADD COLUMN value_secret_id TEXT REFERENCES secrets(id);

-- Environments carry two distinct sets: the TLS/SSH material an operator
-- pastes in, and the agent enrolment token. The kubeconfig and bearer_token
-- columns get no pointer: Kubernetes moves to a plugin and they are dropped.
ALTER TABLE environments ADD COLUMN tls_ca_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE environments ADD COLUMN tls_cert_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE environments ADD COLUMN tls_key_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE environments ADD COLUMN ssh_key_secret_id TEXT REFERENCES secrets(id);
ALTER TABLE environments ADD COLUMN agent_token_secret_id TEXT REFERENCES secrets(id);

CREATE INDEX IF NOT EXISTS idx_webhooks_secret_id ON webhooks(secret_secret_id);
CREATE INDEX IF NOT EXISTS idx_registries_secret_id ON registries(password_secret_id);
CREATE INDEX IF NOT EXISTS idx_git_repos_secret_id ON git_repos(credential_secret_id);
CREATE INDEX IF NOT EXISTS idx_environments_agent_token_secret_id ON environments(agent_token_secret_id);
