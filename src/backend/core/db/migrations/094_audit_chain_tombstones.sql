-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS audit_chain_tombstones (
    id              TEXT PRIMARY KEY,
    entry_hash      TEXT NOT NULL DEFAULT '',
    exit_hash       TEXT NOT NULL,
    row_count       INTEGER NOT NULL DEFAULT 0,
    first_row_id    TEXT NOT NULL DEFAULT '',
    last_row_id     TEXT NOT NULL DEFAULT '',
    first_timestamp TEXT NOT NULL DEFAULT '',
    last_timestamp  TEXT NOT NULL DEFAULT '',
    severities      TEXT NOT NULL DEFAULT '',
    seal            TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_audit_chain_tombstones_entry ON audit_chain_tombstones(entry_hash);
CREATE INDEX IF NOT EXISTS idx_audit_chain_tombstones_created ON audit_chain_tombstones(created_at);
