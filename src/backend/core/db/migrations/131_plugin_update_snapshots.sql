-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS appstore_plugin_update_snapshots (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    version     TEXT NOT NULL,
    build       TEXT NOT NULL DEFAULT '',
    built_for   TEXT NOT NULL DEFAULT '',
    signed      INTEGER NOT NULL DEFAULT 0,
    enabled     INTEGER NOT NULL DEFAULT 0,
    grants      TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
