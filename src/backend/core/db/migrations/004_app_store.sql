-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- McHarbor App Store

-- ─── App Store Catalog ───────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS appstore_catalog (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    image TEXT NOT NULL DEFAULT '',
    logo TEXT NOT NULL DEFAULT '',
    website TEXT NOT NULL DEFAULT '',
    docs_url TEXT NOT NULL DEFAULT '',
    ports TEXT NOT NULL DEFAULT '[]',
    volumes TEXT NOT NULL DEFAULT '[]',
    env_vars TEXT NOT NULL DEFAULT '[]',
    compose_override TEXT NOT NULL DEFAULT '',
    min_memory TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'bundled' CHECK(source IN ('bundled','remote')),
    version TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_appstore_catalog_slug ON appstore_catalog(slug);
CREATE INDEX IF NOT EXISTS idx_appstore_catalog_category ON appstore_catalog(category);

-- ─── App Store Installed ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS appstore_installed (
    id TEXT PRIMARY KEY,
    catalog_slug TEXT NOT NULL,
    stack_id TEXT NOT NULL,
    stack_name TEXT NOT NULL DEFAULT '',
    environment_id TEXT,
    installed_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (stack_id) REFERENCES stacks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_appstore_installed_slug ON appstore_installed(catalog_slug);
CREATE INDEX IF NOT EXISTS idx_appstore_installed_stack ON appstore_installed(stack_id);

-- ─── App Store Sync ──────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS appstore_sync (
    id TEXT PRIMARY KEY,
    last_synced_at TEXT,
    status TEXT NOT NULL DEFAULT 'never' CHECK(status IN ('never','syncing','success','error')),
    error TEXT NOT NULL DEFAULT '',
    apps_added INTEGER NOT NULL DEFAULT 0,
    apps_updated INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
