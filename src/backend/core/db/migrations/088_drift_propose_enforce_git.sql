-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE desired_states ADD COLUMN mode TEXT NOT NULL DEFAULT 'observe';
ALTER TABLE desired_states ADD COLUMN source_id TEXT;
ALTER TABLE desired_states ADD COLUMN source_path TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_states ADD COLUMN source_compose TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_states ADD COLUMN last_enforced_at TEXT;
CREATE INDEX IF NOT EXISTS idx_desired_states_source ON desired_states(source_id);

CREATE TABLE IF NOT EXISTS reconcile_proposals (
    id                TEXT PRIMARY KEY,
    desired_state_id  TEXT NOT NULL,
    env_id            TEXT,
    origin            TEXT NOT NULL DEFAULT 'propose',
    status            TEXT NOT NULL DEFAULT 'pending',
    fingerprint       TEXT NOT NULL DEFAULT '',
    report            TEXT NOT NULL DEFAULT '',
    steps             TEXT NOT NULL DEFAULT '',
    result            TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    note              TEXT NOT NULL DEFAULT '',
    resolved_by       TEXT NOT NULL DEFAULT '',
    resolved_at       TEXT,
    created_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_reconcile_proposals_state ON reconcile_proposals(desired_state_id, status);
CREATE INDEX IF NOT EXISTS idx_reconcile_proposals_status ON reconcile_proposals(status, created_at);
CREATE INDEX IF NOT EXISTS idx_reconcile_proposals_env ON reconcile_proposals(env_id);

CREATE TABLE IF NOT EXISTS reconciler_settings (
    id                    TEXT PRIMARY KEY,
    confirm_passes        INTEGER NOT NULL DEFAULT 2,
    enforce_enabled       INTEGER NOT NULL DEFAULT 0,
    max_actions_per_pass  INTEGER NOT NULL DEFAULT 3,
    cooldown_seconds      INTEGER NOT NULL DEFAULT 600,
    failure_threshold     INTEGER NOT NULL DEFAULT 3,
    breaker_open          INTEGER NOT NULL DEFAULT 0,
    breaker_reason        TEXT NOT NULL DEFAULT '',
    breaker_opened_at     TEXT,
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    created_at            TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at            TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
INSERT INTO reconciler_settings (id) VALUES ('global');

CREATE TABLE IF NOT EXISTS desired_state_sources (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    repo_id         TEXT NOT NULL,
    branch          TEXT NOT NULL DEFAULT 'main',
    path            TEXT NOT NULL DEFAULT '',
    env_id          TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_commit     TEXT NOT NULL DEFAULT '',
    last_synced_at  TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    row_count       INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
CREATE INDEX IF NOT EXISTS idx_desired_state_sources_repo ON desired_state_sources(repo_id);
CREATE INDEX IF NOT EXISTS idx_desired_state_sources_env ON desired_state_sources(env_id);
