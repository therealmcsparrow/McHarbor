-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_cli_roots (
    id         TEXT PRIMARY KEY,
    origin     TEXT NOT NULL,
    name       TEXT NOT NULL,
    short      TEXT NOT NULL DEFAULT '',
    aliases    TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_plugin_cli_roots_origin
    ON plugin_cli_roots(origin);

CREATE TABLE IF NOT EXISTS plugin_cli_commands (
    id         TEXT PRIMARY KEY,
    origin     TEXT NOT NULL,
    path       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    method     TEXT NOT NULL,
    route      TEXT NOT NULL,
    args       TEXT NOT NULL DEFAULT '[]',
    flags      TEXT NOT NULL DEFAULT '[]',
    columns    TEXT NOT NULL DEFAULT '[]',
    body       TEXT NOT NULL DEFAULT 'none',
    needs      TEXT NOT NULL DEFAULT '',
    risk       TEXT NOT NULL DEFAULT 'safe',
    guest_kind TEXT NOT NULL DEFAULT '',
    short      TEXT NOT NULL DEFAULT '',
    long       TEXT NOT NULL DEFAULT '',
    example    TEXT NOT NULL DEFAULT '',
    env_scoped INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE(origin, path)
);
CREATE INDEX IF NOT EXISTS idx_plugin_cli_commands_origin
    ON plugin_cli_commands(origin);
