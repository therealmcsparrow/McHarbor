-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS install_tokens (
    id TEXT PRIMARY KEY,
    env_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_install_tokens_env_id ON install_tokens(env_id);
CREATE INDEX idx_install_tokens_token_hash ON install_tokens(token_hash);
