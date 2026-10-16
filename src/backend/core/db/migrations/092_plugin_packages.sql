-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_packages (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL,
    version     TEXT NOT NULL,
    build       TEXT NOT NULL DEFAULT '',
    sha256      TEXT NOT NULL,
    size        INTEGER NOT NULL DEFAULT 0,
    signature   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_packages_slug_version ON plugin_packages(slug, version);

CREATE TABLE IF NOT EXISTS plugin_package_chunks (
    id          TEXT PRIMARY KEY,
    package_id  TEXT NOT NULL,
    seq         INTEGER NOT NULL,
    data        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_plugin_package_chunks_package ON plugin_package_chunks(package_id, seq);
