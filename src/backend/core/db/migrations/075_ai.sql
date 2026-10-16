-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- The AI assistant's own tables.

-- AI assistant conversation history.
CREATE TABLE IF NOT EXISTS ai_assistant_conversations (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    environment_id TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_ai_assistant_conv_user ON ai_assistant_conversations(user_id);

CREATE TABLE IF NOT EXISTS ai_assistant_messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES ai_assistant_conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('user','assistant','system')),
    content TEXT NOT NULL,
    context TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_ai_assistant_msg_conv ON ai_assistant_messages(conversation_id);


-- AI provider connections: the host-owned CRUD table for named LLM
-- provider instances. Mirrors the storage_locations / communication_channels
-- pattern. One row = one user-named connection (e.g. "OpenAI – Production",
-- "OpenAI – Dev"). Multiple rows may share a provider_type; the chat
-- composer selects by connection id and falls back to the row whose
-- is_default = 1.
--
-- `api_key` is encrypted at rest with AES-256-GCM via core/encryption.
-- `base_url` and `default_model` are plaintext so the operator can audit
-- them in the DB if needed. `enabled` is the runtime gate; `is_default` is
-- single-row at the service layer (cleared atomically on SetDefault, see
-- ai_providers.Update).
--
-- Second authentication method: an OAuth 2.0 Device Authorization Grant
-- (RFC 8628) "sign in" flow, selectable per connection as an alternative to
-- a pasted API key. `auth_method` defaults to 'api_key'.
--
-- Device flow, not authorization-code: McHarbor is self-hosted and normally
-- has no publicly reachable redirect URL, while the device flow only needs
-- outbound HTTPS.
--
-- Column notes:
--   * `oauth_client_secret`, `oauth_access_token`, `oauth_refresh_token` and
--     `oauth_pending_device_code` hold ciphertext from `encryption.Service`
--     (AES-256-GCM), exactly like `api_key`. None of them is ever serialized
--     to a client -- the read path returns booleans and the account label
--     only.
--   * The `oauth_pending_*` columns are scanned into plain `string`, so they
--     are NOT NULL DEFAULT '' (a NULL there is a scan error at runtime). The
--     columns scanned into `sql.NullString` are left nullable.
--   * Timestamps are RFC3339 TEXT, matching every other table here.

CREATE TABLE IF NOT EXISTS ai_provider_connections (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    provider_type   TEXT NOT NULL,
    is_default      INTEGER NOT NULL DEFAULT 0,
    enabled         INTEGER NOT NULL DEFAULT 1,
    base_url        TEXT,
    default_model   TEXT,
    api_key         TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    auth_method     TEXT NOT NULL DEFAULT 'api_key',
    oauth_client_id TEXT,
    oauth_client_secret TEXT,
    oauth_device_url TEXT,
    oauth_token_url TEXT,
    oauth_scopes    TEXT,
    oauth_access_token TEXT,
    oauth_refresh_token TEXT,
    oauth_token_type TEXT NOT NULL DEFAULT 'Bearer',
    oauth_expires_at TEXT,
    oauth_account_label TEXT,
    oauth_pending_device_code TEXT NOT NULL DEFAULT '',
    oauth_pending_user_code TEXT NOT NULL DEFAULT '',
    oauth_pending_verification_uri TEXT NOT NULL DEFAULT '',
    oauth_pending_expires_at TEXT NOT NULL DEFAULT '',
    oauth_pending_interval INTEGER NOT NULL DEFAULT 0,
    api_key_secret_id TEXT REFERENCES secrets(id),
    oauth_client_secret_secret_id TEXT REFERENCES secrets(id)
);

CREATE INDEX IF NOT EXISTS idx_ai_provider_connections_type ON ai_provider_connections(provider_type);
CREATE INDEX IF NOT EXISTS idx_ai_provider_connections_enabled ON ai_provider_connections(enabled);

-- Only one row may be the default. Partial unique index keeps the
-- "exactly one default" invariant at the DB layer so a race in the
-- service's "clear then set" transaction can never leave two rows
-- marked default. Application code MUST clear the existing default
-- inside the same transaction as the new insert/update.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_provider_connections_default
  ON ai_provider_connections(is_default) WHERE is_default = 1;


-- Per-user daily counters used by the AI assistant usage-cap middleware.
CREATE TABLE IF NOT EXISTS ai_assistant_usage (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    daily_window_start TEXT NOT NULL,
    daily_turns INTEGER NOT NULL DEFAULT 0,
    daily_tool_calls INTEGER NOT NULL DEFAULT 0,
    daily_action_calls INTEGER NOT NULL DEFAULT 0,
    daily_tokens_in INTEGER NOT NULL DEFAULT 0,
    daily_tokens_out INTEGER NOT NULL DEFAULT 0,
    last_turn_at TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);


-- Consent-gated AI assistant tool calls. Rows start as pending after a
-- dry-run preview, then move through approved/denied/executed/failed.
CREATE TABLE IF NOT EXISTS ai_assistant_action_log (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES ai_assistant_conversations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tool TEXT NOT NULL,
    args_json TEXT NOT NULL DEFAULT '{}',
    plan_json TEXT NOT NULL DEFAULT '{}',
    result_json TEXT NOT NULL DEFAULT '',
    consent_at TEXT,
    executed_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    idempotency_key TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','executed','failed','denied','expired')),
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_ai_assistant_action_user
    ON ai_assistant_action_log(user_id);

CREATE INDEX IF NOT EXISTS idx_ai_assistant_action_conversation
    ON ai_assistant_action_log(conversation_id);

CREATE INDEX IF NOT EXISTS idx_ai_assistant_action_status
    ON ai_assistant_action_log(status);

CREATE INDEX IF NOT EXISTS idx_ai_assistant_action_executed
    ON ai_assistant_action_log(executed_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_assistant_action_idempotency
    ON ai_assistant_action_log(user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key != '';
