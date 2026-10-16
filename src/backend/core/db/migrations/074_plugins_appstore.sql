-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- The plugin system: installed plugins, the app and plugin catalogs, and
-- every contribution table the runtime writes to at boot.

-- installed_by points at `__legacy_owner__` (066_accounts.sql) for plugins
-- installed before ownership was tracked.
--
-- installed_as_dependency: a plugin manifest may declare
-- `requires: [<slug>, ...]`. The installer pulls each missing dependency in
-- alongside it, and removes one again when the last plugin that required it
-- is uninstalled. The flag is what makes that removal safe. Without it, an
-- operator who installed Compute Foundation deliberately, then tried the KVM
-- plugin and removed it again, would silently lose the plugin they chose
-- themselves. A dependency pulled in by the installer is the host's to clean
-- up; one the operator asked for is not, and the two are otherwise
-- indistinguishable.
--
-- signed: third-party plugins are installable without a signature from a
-- pinned trust key — refusing them outright left an operator with no way to
-- run anything the project did not sign, and the installer already warns
-- before it extracts. What the refusal did provide was certainty about what
-- is running, so the trust decision is recorded per install rather than
-- discarded. 1 = the tarball carried a valid ed25519 signature from a
-- configured trust key at install time. 0 = it did not, and the operator
-- accepted the risk.

CREATE TABLE IF NOT EXISTS appstore_plugins (
    id            TEXT PRIMARY KEY,
    slug          TEXT NOT NULL,
    name          TEXT NOT NULL,
    version       TEXT NOT NULL,
    author        TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    homepage      TEXT NOT NULL DEFAULT '',
    license       TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT 'bundled', -- 'bundled' | 'plugin'
    signature     TEXT NOT NULL DEFAULT '',
    checksum      TEXT NOT NULL DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 0,
    installed_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    installed_by  TEXT REFERENCES users(id) ON DELETE SET NULL,
    installed_as_dependency INTEGER NOT NULL DEFAULT 0,
    signed        INTEGER NOT NULL DEFAULT 0,
    built_for     TEXT NOT NULL DEFAULT '',
    repair_error  TEXT NOT NULL DEFAULT '',
    build         TEXT,
    UNIQUE(slug, version)
);

CREATE INDEX IF NOT EXISTS idx_appstore_plugins_slug         ON appstore_plugins(slug);
CREATE INDEX IF NOT EXISTS idx_appstore_plugins_enabled      ON appstore_plugins(enabled);
CREATE INDEX IF NOT EXISTS idx_appstore_plugins_installed_by ON appstore_plugins(installed_by);
CREATE INDEX IF NOT EXISTS idx_appstore_plugins_built_for    ON appstore_plugins(built_for);


-- Plugin UI hook points: topbar items, per-page context menus, and
-- per-page tabs. The sidemenu (plugin_menu) is created with the other
-- contribution tables below; these are the remaining three surfaces
-- documented in .claude/skills/plugin/SKILL.md.
--
-- Each row is owned by exactly one plugin slug (origin column) and
-- is bulk-deleted by origin in the uninstall gate chain (installer
-- step 4). The plugin_runtime service reads these tables via the
-- /api/plugins/loader/{topbar,context-menus,tabs} endpoints, which
-- the SPA loader fetches in parallel alongside /menu, /widgets, /nodes.
--
-- page_id column on plugin_context_menus and plugin_tabs is indexed
-- because the host registry's hot-path is keyed by host page id (e.g.
-- "container-detail"); see core/plugins/pageTabsRegistry.ts and
-- core/plugins/contextMenuRegistry.ts on the SPA side.

CREATE TABLE IF NOT EXISTS plugin_topbar (
    id          TEXT PRIMARY KEY,
    origin      TEXT NOT NULL,
    slot        TEXT NOT NULL DEFAULT 'left',     -- 'left' | 'right'
    label_ns    TEXT NOT NULL,
    label_key   TEXT NOT NULL,
    icon        TEXT NOT NULL DEFAULT '',
    path        TEXT NOT NULL DEFAULT '',
    visibility  TEXT NOT NULL DEFAULT 'always',   -- 'always' | 'permission:<key>'
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_plugin_topbar_origin ON plugin_topbar(origin);

CREATE TABLE IF NOT EXISTS plugin_context_menus (
    id                  TEXT PRIMARY KEY,
    origin              TEXT NOT NULL,
    page_id             TEXT NOT NULL,             -- host page id, e.g. 'container-detail'
    label_ns            TEXT NOT NULL,
    label_key           TEXT NOT NULL,
    icon                TEXT NOT NULL DEFAULT '',
    when_expr           TEXT NOT NULL DEFAULT '',  -- optional JS-boolean expression
    requires_permission TEXT NOT NULL DEFAULT '',
    sort_order          INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_plugin_context_menus_origin
    ON plugin_context_menus(origin);
CREATE INDEX IF NOT EXISTS idx_plugin_context_menus_page
    ON plugin_context_menus(page_id, sort_order);

CREATE TABLE IF NOT EXISTS plugin_tabs (
    id                  TEXT PRIMARY KEY,
    origin              TEXT NOT NULL,
    page_id             TEXT NOT NULL,             -- host page id
    tab_key             TEXT NOT NULL,             -- unique per (origin, page_id)
    label_ns            TEXT NOT NULL,
    label_key           TEXT NOT NULL,
    icon                TEXT NOT NULL DEFAULT '',
    requires_permission TEXT NOT NULL DEFAULT '',
    sort_order          INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(origin, page_id, tab_key)
);
CREATE INDEX IF NOT EXISTS idx_plugin_tabs_origin
    ON plugin_tabs(origin);
CREATE INDEX IF NOT EXISTS idx_plugin_tabs_page
    ON plugin_tabs(page_id, sort_order);


-- plugin_nodes carries per-node config_schema / input_ports / output_ports
-- metadata so the workflow canvas can render plugin nodes with real ports
-- and a real config UI.
--
-- JSON column choice: matches the pattern used by container_backups.
-- snapshot_config and similar plugin-owned JSON columns — keeps the
-- schema to one column per concern (config / ports) instead of
-- seven individual columns, and lets the host evolve the shape
-- (e.g. add a port type) without another ALTER.

CREATE TABLE IF NOT EXISTS plugin_nodes (
    id                  TEXT PRIMARY KEY,
    origin              TEXT NOT NULL,
    key                 TEXT NOT NULL,
    category            TEXT NOT NULL DEFAULT '',
    display_name        TEXT NOT NULL DEFAULT '',
    icon                TEXT NOT NULL DEFAULT '',
    config_schema_json  TEXT NOT NULL DEFAULT '[]',
    input_ports_json    TEXT NOT NULL DEFAULT '[]',
    output_ports_json   TEXT NOT NULL DEFAULT '[]',
    requires_permission TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(origin, key)
);
CREATE INDEX IF NOT EXISTS idx_plugin_nodes_origin ON plugin_nodes(origin);


-- The contribution tables the plugin runtime loader writes to when a plugin
-- calls RegisterRoutes / RegisterMenu / RegisterWidgets at boot.
--
-- Without these tables every plugin load fails on the very first
-- RegisterRoutes call (`SQL logic error: no such table: plugin_routes`),
-- so the per-plugin recover fires and RegisterAdapters never runs.
--
-- The schemas mirror the columns the runtime code expects (see
-- modules/appstore/plugins/service.go RecordRoute/RecordMenu/RecordWidget
-- and modules/plugin_runtime/service.go ListRoutes/ListMenu/ListWidgets).
-- All three tables share the `origin` slug column the runtime uses for
-- per-plugin unload, plus an index on (origin, sort_order/path/key) for
-- the ordered list queries the loader endpoints serve.
--
-- plugin_menu.priority: sidebar entries were ordered by
-- `origin ASC, sort_order ASC`, and sort_order is derived per plugin from how
-- many rows that plugin already has, so the order inside a section was
-- alphabetical by plugin slug. priority is the explicit answer: higher sorts
-- first, and the default of 0 leaves every entry that says nothing exactly
-- where it was.
--
-- plugin_widgets.settings_json: the descriptor's free-form Settings map
-- (description, icon, min_width, min_height) travels with the row, so a
-- plugin widget in the picker has more than a puzzle-piece icon, no
-- description and a fixed 2x2 minimum.

CREATE TABLE IF NOT EXISTS plugin_routes (
    id                    TEXT PRIMARY KEY,
    origin                TEXT NOT NULL,           -- plugin slug
    method                TEXT NOT NULL,           -- 'GET' | 'POST' | etc.
    path                  TEXT NOT NULL,           -- '/api/foo/bar'
    requires_permission   TEXT NOT NULL DEFAULT '',
    is_public             INTEGER NOT NULL DEFAULT 0,  -- boolean: 0/1
    created_at            TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_plugin_routes_origin
    ON plugin_routes(origin);
CREATE INDEX IF NOT EXISTS idx_plugin_routes_path
    ON plugin_routes(path);

CREATE TABLE IF NOT EXISTS plugin_menu (
    id                    TEXT PRIMARY KEY,
    origin                TEXT NOT NULL,
    parent                TEXT NOT NULL DEFAULT '', -- optional section id
    label_ns              TEXT NOT NULL,            -- i18n namespace, e.g. 'plugins'
    label_key             TEXT NOT NULL,            -- i18n key under the namespace
    path                  TEXT NOT NULL DEFAULT '',
    icon                  TEXT NOT NULL DEFAULT '',
    permission            TEXT NOT NULL DEFAULT '',
    sort_order            INTEGER NOT NULL DEFAULT 0,
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    priority              INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_plugin_menu_origin
    ON plugin_menu(origin);
CREATE INDEX IF NOT EXISTS idx_plugin_menu_order
    ON plugin_menu(origin, sort_order);

CREATE TABLE IF NOT EXISTS plugin_widgets (
    origin                TEXT NOT NULL,
    key                   TEXT NOT NULL,            -- widget key within the plugin
    title                 TEXT NOT NULL DEFAULT '',
    category              TEXT NOT NULL DEFAULT '',
    default_size          TEXT NOT NULL DEFAULT '4x3',  -- 'WxH' grid spec
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    settings_json         TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (origin, key)
);
CREATE INDEX IF NOT EXISTS idx_plugin_widgets_origin
    ON plugin_widgets(origin);


-- Plugin-contributed themes. CSS paths are plugin-relative asset paths
-- such as themes/my-theme.css and are served through the plugin loader.
CREATE TABLE IF NOT EXISTS plugin_themes (
    origin          TEXT NOT NULL,
    theme_id        TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    css_path        TEXT NOT NULL DEFAULT '',
    swatch_hue      INTEGER NOT NULL DEFAULT 200,
    default_variant TEXT NOT NULL DEFAULT 'dark' CHECK(default_variant IN ('light', 'dark')),
    has_dark        INTEGER NOT NULL DEFAULT 1,
    has_light       INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (origin, theme_id)
);

CREATE INDEX IF NOT EXISTS idx_plugin_themes_origin
    ON plugin_themes(origin);


-- The plugin key/value config store.
--
-- Call sites that assume this table exists:
--
--   core/pluginruntime/loader.go        kvAdapter.Get / Set / Delete —
--                                       the `plugin.KVStore` handed to
--                                       every plugin's Init().
--   modules/appstore/plugins/
--     installer_default.go              writes the configure-form values
--                                       on install (is_secret = 1, the
--                                       value already AES-256-GCM
--                                       encrypted by the caller).
--     service.go                        ListConfig / DeleteConfig.
--   modules/ai_assistant/
--     plugin_dispatch_bridge.go         reads a provider plugin's
--                                       api_key / base_url / model.
--
-- Shape notes:
--
--   * The primary key is (origin, key), not an xid `id`. Every writer
--     upserts with `ON CONFLICT (origin, key) DO UPDATE`, which SQLite
--     resolves only against a real PRIMARY KEY or UNIQUE constraint on
--     exactly those columns. plugin_permissions is the precedent — it is
--     the other per-origin lookup table and carries the same composite
--     key with no surrogate id.
--   * value_enc holds ciphertext when is_secret = 1 and plaintext when
--     is_secret = 0. kvAdapter.Get filters on `is_secret = 0` so a
--     plugin's own KV can never read back a host-encrypted secret it has
--     no key for.
--   * Rows outlive an uninstall/reinstall of the same slug on purpose —
--     see the note at modules/appstore/plugins/handler.go:248.

CREATE TABLE IF NOT EXISTS plugin_config (
    origin      TEXT    NOT NULL,                          -- plugin slug
    key         TEXT    NOT NULL,
    value_enc   TEXT    NOT NULL DEFAULT '',               -- ciphertext when is_secret = 1
    is_secret   INTEGER NOT NULL DEFAULT 0,                -- boolean: 0/1
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (origin, key)
);

CREATE INDEX IF NOT EXISTS idx_plugin_config_origin
    ON plugin_config(origin);


-- Per-adapter notification capabilities, recorded from the plugin's own
-- spec at load time.
--
-- Adapter capabilities otherwise exist only in the in-memory
-- adapterRegistry, which means nothing outside the backend process could
-- ask "can this channel carry a file?" without holding the registry.
-- The reporting scheduler needs exactly that question answered from the
-- UI, so the flags are mirrored here alongside the other plugin
-- contribution tables (plugin_menu, plugin_routes, plugin_widgets).
--
-- Rows are cleared and rewritten on every plugin load, not only at
-- install, so a capability that flips in a rebuilt plugin is picked up
-- without a reinstall.
--
-- The recipient_* columns mirror each adapter's declared overridable
-- destination: "may an operator send this one report somewhere other than
-- the channel's configured destination?" - true for an SMTP `to` or a
-- Telegram `chat_id`, false for a Slack webhook whose URL is both the
-- destination and the secret. recipient_key empty is the answer "no", and is
-- the default, so every adapter that has not been rebuilt against the
-- capability keeps behaving as before.
CREATE TABLE IF NOT EXISTS plugin_notification_adapters (
    origin                    TEXT NOT NULL,           -- plugin slug
    channel_type              TEXT NOT NULL,           -- 'discord', 'slack', …
    display_name              TEXT NOT NULL DEFAULT '',
    supports_file_attachments INTEGER NOT NULL DEFAULT 0,
    supports_rich_attachments INTEGER NOT NULL DEFAULT 0,
    encrypted_credentials     INTEGER NOT NULL DEFAULT 0,
    created_at                TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    recipient_key             TEXT NOT NULL DEFAULT '',
    recipient_kind            TEXT NOT NULL DEFAULT '',
    recipient_label           TEXT NOT NULL DEFAULT '',
    recipient_placeholder     TEXT NOT NULL DEFAULT '',
    recipient_multiple        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (origin, channel_type)
);

CREATE INDEX IF NOT EXISTS idx_plugin_notification_adapters_type
    ON plugin_notification_adapters(channel_type);


-- Per-kind catalog registry. Admins add catalogs (Docker apps, plugins,
-- VM images) via Settings → Catalogs. Built-in defaults are seeded by
-- SeedDefaults with `readonly = 1` and cannot be deleted.

CREATE TABLE IF NOT EXISTS settings_catalogs (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    kind       TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    readonly   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    origin     TEXT NOT NULL DEFAULT '',
    UNIQUE(kind, url)
);

CREATE INDEX IF NOT EXISTS idx_settings_catalogs_kind ON settings_catalogs(kind);
CREATE INDEX IF NOT EXISTS idx_settings_catalogs_origin
    ON settings_catalogs(origin);

CREATE TABLE IF NOT EXISTS plugin_catalogs (
    origin     TEXT NOT NULL,
    catalog_id TEXT NOT NULL,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    url        TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (origin, catalog_id)
);

CREATE INDEX IF NOT EXISTS idx_plugin_catalogs_origin
    ON plugin_catalogs(origin);

CREATE INDEX IF NOT EXISTS idx_plugin_catalogs_kind
    ON plugin_catalogs(kind);

CREATE TABLE IF NOT EXISTS plugin_migrations (
    id TEXT PRIMARY KEY,
    origin TEXT NOT NULL,
    filename TEXT NOT NULL,
    applied_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_migrations_origin_filename
    ON plugin_migrations(origin, filename);

CREATE INDEX IF NOT EXISTS idx_plugin_migrations_origin
    ON plugin_migrations(origin);


-- Docker application definitions now come from settings_catalogs sources.
-- Rebuild the table to replace the legacy "bundled" source value with
-- "catalog".
--
-- latest_version / latest_release_date: every app definition in the catalog
-- carries a "latest" block —
--
--   "latest": { "version": "0.107.65", "release_date": "2026-08-21" }
--
-- The catalog server owns those two values — it resolves the real release, not
-- the "latest" tag — and the App Store card reads its "Latest version" and
-- "Updated" columns from them. They are catalog facts, so they live on the
-- catalog row rather than in appstore_registry_meta, which stays a cache of
-- what a registry happens to publish and is only the fallback.

ALTER TABLE appstore_catalog RENAME TO appstore_catalog_legacy;

CREATE TABLE appstore_catalog (
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
    source TEXT NOT NULL DEFAULT 'catalog' CHECK(source IN ('catalog','remote')),
    version TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    latest_version TEXT NOT NULL DEFAULT '',
    latest_release_date TEXT NOT NULL DEFAULT ''
);

INSERT INTO appstore_catalog (
    id, slug, name, description, category, image, logo, website, docs_url,
    ports, volumes, env_vars, compose_override, min_memory, source, version,
    created_at, updated_at
)
SELECT
    id, slug, name, description, category, image, logo, website, docs_url,
    ports, volumes, env_vars, compose_override, min_memory,
    CASE WHEN source = 'bundled' THEN 'catalog' ELSE source END,
    version, created_at, updated_at
FROM appstore_catalog_legacy;

DROP TABLE appstore_catalog_legacy;

CREATE UNIQUE INDEX idx_appstore_catalog_slug ON appstore_catalog(slug);
CREATE INDEX idx_appstore_catalog_category ON appstore_catalog(category);


-- The App Store "Installed" flag is one row in appstore_installed per install.
--
-- Migration 004 tied that row to the stack row with
-- `FOREIGN KEY (stack_id) REFERENCES stacks(id) ON DELETE CASCADE`, and SQLite
-- runs with `PRAGMA foreign_keys=ON`, so *any* delete of the stacks row took the
-- installation record with it — silently, and regardless of whether the app was
-- still running. That is why an installed app could lose its badge while its
-- containers were up.
--
-- Ownership of the record belongs with the App Store, not with the stacks table:
-- the record is removed when the store uninstalls the app, when the stack is
-- deleted by hand, and when the app's containers are gone from the environment
-- (reconciled against live Docker/agent state). All three happen explicitly
-- in Go, so the cascade only ever removed the record behind the store's back.
--
-- stack_id stays as a soft reference — it still points at the stack the install
-- created, it just no longer dictates the record's lifetime.

PRAGMA foreign_keys = OFF;

CREATE TABLE appstore_installed_new (
    id TEXT PRIMARY KEY,
    catalog_slug TEXT NOT NULL,
    stack_id TEXT NOT NULL,
    stack_name TEXT NOT NULL DEFAULT '',
    environment_id TEXT,
    installed_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO appstore_installed_new
    (id, catalog_slug, stack_id, stack_name, environment_id, installed_at, created_at, updated_at)
SELECT
     id, catalog_slug, stack_id, stack_name, environment_id, installed_at, created_at, updated_at
FROM appstore_installed;

DROP TABLE appstore_installed;
ALTER TABLE appstore_installed_new RENAME TO appstore_installed;

CREATE INDEX idx_appstore_installed_slug ON appstore_installed(catalog_slug);
CREATE INDEX idx_appstore_installed_stack ON appstore_installed(stack_id);
CREATE INDEX idx_appstore_installed_env ON appstore_installed(environment_id);

PRAGMA foreign_keys = ON;


-- App Store cards claim a "Latest" version and an "Updated" date. When the
-- catalog does not supply them, this table caches what the registry actually
-- publishes for an image reference — the newest release tag and when that
-- tag was pushed — so the card reports the distribution, not the catalog. It
-- is a cache: every row is re-derivable from the registry, so a failed lookup
-- is recorded (with its checked_at) rather than retried on every page view.

CREATE TABLE IF NOT EXISTS appstore_registry_meta (
    id             TEXT PRIMARY KEY,
    -- The catalog image reference, verbatim (e.g. "ghcr.io/owner/app:1.2.3").
    image_ref      TEXT NOT NULL,
    -- Newest release tag the registry lists, normalised (no leading "v").
    latest_version TEXT NOT NULL DEFAULT '',
    -- When that tag was last pushed/built, RFC3339.
    released_at    TEXT NOT NULL DEFAULT '',
    -- 'ok' or 'error'. An error row still holds checked_at so a registry that
    -- rate-limits or hides its tag list is not asked again on every request.
    status         TEXT NOT NULL DEFAULT '' CHECK(status IN ('', 'ok', 'error')),
    error          TEXT NOT NULL DEFAULT '',
    checked_at     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_appstore_registry_meta_ref
    ON appstore_registry_meta(image_ref);
CREATE INDEX IF NOT EXISTS idx_appstore_registry_meta_checked
    ON appstore_registry_meta(checked_at);


-- Audit action catalog: the work plugins are asked to do.
--
-- The host audits every adapter call it dispatches to a plugin
-- (core/pluginruntime/adapter_audit.go); these rows give the Actions
-- dialog a description for each one, and give a row whose severity was
-- not set explicitly a sensible default.

INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES
    ('audit_action_plugin_001', 'plugins', 'plugin.adapter.storage.test', 'info', 'Storage destination connection was tested through a plugin adapter'),
    ('audit_action_plugin_002', 'plugins', 'plugin.adapter.storage.upload', 'success', 'File was uploaded to a storage destination through a plugin adapter'),
    ('audit_action_plugin_003', 'plugins', 'plugin.adapter.storage.download', 'info', 'File download from a storage destination failed in a plugin adapter'),
    ('audit_action_plugin_004', 'plugins', 'plugin.adapter.storage.delete', 'warning', 'File was deleted from a storage destination through a plugin adapter'),
    ('audit_action_plugin_005', 'plugins', 'plugin.adapter.storage.list', 'info', 'Storage destination listing failed in a plugin adapter'),
    ('audit_action_plugin_006', 'plugins', 'plugin.adapter.storage.consent_url', 'info', 'OAuth consent URL was built for a storage destination by a plugin adapter'),
    ('audit_action_plugin_007', 'plugins', 'plugin.adapter.storage.consent_callback', 'success', 'OAuth consent was completed for a storage destination by a plugin adapter'),

    ('audit_action_plugin_008', 'plugins', 'plugin.adapter.notification.test', 'info', 'Communication channel was tested through a plugin adapter'),
    ('audit_action_plugin_009', 'plugins', 'plugin.adapter.notification.send', 'success', 'Message was sent to a communication channel through a plugin adapter'),

    ('audit_action_plugin_010', 'plugins', 'plugin.adapter.identity.test', 'info', 'Identity provider connection was tested through a plugin adapter'),
    ('audit_action_plugin_011', 'plugins', 'plugin.adapter.identity.begin_auth', 'info', 'Single sign-on flow was started by a plugin adapter'),
    ('audit_action_plugin_012', 'plugins', 'plugin.adapter.identity.callback', 'success', 'Single sign-on callback was handled by a plugin adapter'),
    ('audit_action_plugin_013', 'plugins', 'plugin.adapter.identity.fetch_groups', 'info', 'Upstream group fetch failed in a plugin adapter'),
    ('audit_action_plugin_014', 'plugins', 'plugin.adapter.identity.metadata', 'info', 'SAML metadata generation failed in a plugin adapter'),

    ('audit_action_plugin_015', 'plugins', 'plugin.adapter.ai.test', 'info', 'AI provider connection was tested through a plugin adapter'),
    ('audit_action_plugin_016', 'plugins', 'plugin.adapter.ai.complete', 'success', 'Chat completion was sent to an AI provider through a plugin adapter'),
    ('audit_action_plugin_017', 'plugins', 'plugin.adapter.ai.models', 'info', 'AI provider model listing failed in a plugin adapter'),

    ('audit_action_plugin_018', 'plugins', 'plugin.adapter.snapshot.capability', 'info', 'Snapshot driver probe failed in a plugin adapter'),
    ('audit_action_plugin_019', 'plugins', 'plugin.adapter.snapshot.create', 'success', 'Filesystem snapshot was created through a plugin adapter'),
    ('audit_action_plugin_020', 'plugins', 'plugin.adapter.snapshot.remove', 'warning', 'Filesystem snapshot was removed through a plugin adapter'),

    ('audit_action_plugin_021', 'plugins', 'plugin.adapter.logging.test', 'info', 'Log forwarder sink was tested through a plugin adapter'),
    ('audit_action_plugin_022', 'plugins', 'plugin.adapter.logging.forward', 'error', 'Audit log forwarding cycle failed in a plugin adapter'),
    ('audit_action_plugin_023', 'plugins', 'plugin.adapter.logging.delivery_history', 'info', 'Log forwarder delivery history read failed in a plugin adapter'),

    ('audit_action_plugin_024', 'plugins', 'plugin.adapter.alerting.evaluate', 'error', 'Alert source evaluation failed in a plugin adapter'),

    ('audit_action_plugin_025', 'plugins', 'plugin.adapter.reporting.build', 'success', 'Report was built by a plugin report provider'),
    ('audit_action_plugin_026', 'plugins', 'plugin.adapter.reporting.subjects', 'info', 'Report subject listing failed in a plugin adapter'),

    ('audit_action_plugin_027', 'plugins', 'plugin.adapter.scanner.test', 'info', 'Image scanner was tested through a plugin adapter'),
    ('audit_action_plugin_028', 'plugins', 'plugin.adapter.scanner.scan', 'success', 'Image was scanned through a plugin adapter')
ON CONFLICT(action) DO NOTHING;

INSERT OR IGNORE INTO audit_action_catalog (id, module, action, severity_default, description) VALUES
('audit_action_addon_001', 'plugins', 'plugin.compute.environment.updated', 'info', 'Compute was enabled, disabled or reconfigured on a host'),
('audit_action_addon_002', 'plugins', 'plugin.compute.pool.created', 'success', 'Storage pool was created'),
('audit_action_addon_003', 'plugins', 'plugin.compute.pool.deleted', 'warning', 'Storage pool was removed, possibly with guest volumes still on it'),
('audit_action_addon_004', 'plugins', 'plugin.compute.pool.moving', 'warning', 'Storage pool relocation was started, which moves guest disks on disk'),
('audit_action_addon_005', 'plugins', 'plugin.compute.pool.moved', 'success', 'Storage pool relocation finished and every disk reached the new location'),
('audit_action_addon_006', 'plugins', 'plugin.compute.pool.move_failed', 'error', 'Storage pool relocation failed part-way and disks may be split across locations'),
('audit_action_addon_007', 'plugins', 'plugin.compute.pool.repaired', 'info', 'Storage pool links were repaired after a relocation'),
('audit_action_addon_008', 'plugins', 'plugin.compute.network.created', 'success', 'Guest network was created'),
('audit_action_addon_009', 'plugins', 'plugin.compute.network.deleted', 'warning', 'Guest network was removed'),
('audit_action_addon_010', 'plugins', 'plugin.compute.catalog.reloaded', 'info', 'Guest image catalog was re-read from disk'),
('audit_action_addon_011', 'plugins', 'plugin.compute.image.added', 'success', 'Guest image was added to the catalog'),
('audit_action_addon_012', 'plugins', 'plugin.compute.image.uploaded', 'success', 'Guest image file was uploaded from the browser onto a host'),
('audit_action_addon_013', 'plugins', 'plugin.compute.image.fetched', 'info', 'Guest image was downloaded to a host'),
('audit_action_addon_014', 'plugins', 'plugin.compute.image.fetched_all', 'info', 'Guest image was downloaded to every compute host'),
('audit_action_addon_015', 'plugins', 'plugin.compute.image.evicted', 'warning', 'Cached guest image was evicted from a host'),
('audit_action_addon_016', 'plugins', 'plugin.compute.image.deleted', 'warning', 'Guest image was removed from the catalog'),

('audit_action_addon_020', 'plugins', 'plugin.kvm.vm.created', 'success', 'Virtual machine was defined'),
('audit_action_addon_021', 'plugins', 'plugin.kvm.vm.updated', 'info', 'Virtual machine settings were changed'),
('audit_action_addon_022', 'plugins', 'plugin.kvm.vm.deleted', 'warning', 'Virtual machine was destroyed, optionally with its volumes'),
('audit_action_addon_023', 'plugins', 'plugin.kvm.vm.start', 'success', 'Virtual machine was started'),
('audit_action_addon_024', 'plugins', 'plugin.kvm.vm.stop', 'warning', 'Virtual machine was powered off without a guest shutdown'),
('audit_action_addon_025', 'plugins', 'plugin.kvm.vm.shutdown', 'warning', 'Virtual machine was asked to shut down, possibly escalating to a power off'),
('audit_action_addon_026', 'plugins', 'plugin.kvm.vm.reset', 'warning', 'Virtual machine was reset, which is a hard reboot of the guest'),
('audit_action_addon_027', 'plugins', 'plugin.kvm.vm.pause', 'warning', 'Virtual machine vCPUs were paused'),
('audit_action_addon_028', 'plugins', 'plugin.kvm.vm.resume', 'success', 'Virtual machine vCPUs were resumed'),
('audit_action_addon_029', 'plugins', 'plugin.kvm.vm.cloned', 'success', 'Virtual machine was cloned'),
('audit_action_addon_030', 'plugins', 'plugin.kvm.vm.templated', 'success', 'Virtual machine was converted into a template'),

('audit_action_addon_031', 'plugins', 'plugin.kvm.disk.added', 'success', 'Disk was attached to a virtual machine'),
('audit_action_addon_032', 'plugins', 'plugin.kvm.disk.resized', 'info', 'Virtual machine disk was resized'),
('audit_action_addon_033', 'plugins', 'plugin.kvm.disk.deleted', 'warning', 'Disk was detached from a virtual machine and its volume removed'),
('audit_action_addon_034', 'plugins', 'plugin.kvm.disk.bus_changed', 'info', 'Virtual machine disk was moved to a different bus'),
('audit_action_addon_035', 'plugins', 'plugin.kvm.media.attached', 'info', 'Disc image was inserted into a virtual machine'),
('audit_action_addon_036', 'plugins', 'plugin.kvm.media.ejected', 'info', 'Disc image was ejected from a virtual machine'),
('audit_action_addon_037', 'plugins', 'plugin.kvm.guest_agent_disc.attached', 'info', 'Guest agent disc was inserted into a virtual machine'),
('audit_action_addon_038', 'plugins', 'plugin.kvm.nic.added', 'success', 'Network interface was attached to a virtual machine'),
('audit_action_addon_039', 'plugins', 'plugin.kvm.nic.deleted', 'warning', 'Network interface was detached from a virtual machine'),
('audit_action_addon_040', 'plugins', 'plugin.kvm.device.attached', 'warning', 'Host device was passed through to a virtual machine'),
('audit_action_addon_041', 'plugins', 'plugin.kvm.device.detached', 'info', 'Host device passthrough was removed from a virtual machine'),

('audit_action_addon_042', 'plugins', 'plugin.kvm.snapshot.created', 'success', 'Virtual machine snapshot was taken'),
('audit_action_addon_043', 'plugins', 'plugin.kvm.snapshot.reverted', 'warning', 'Virtual machine was reverted to a snapshot, discarding everything written since'),
('audit_action_addon_044', 'plugins', 'plugin.kvm.snapshot.deleted', 'warning', 'Virtual machine snapshot was removed'),
('audit_action_addon_045', 'plugins', 'plugin.kvm.snapshot_schedule.created', 'success', 'Snapshot schedule was created'),
('audit_action_addon_046', 'plugins', 'plugin.kvm.snapshot_schedule.updated', 'info', 'Snapshot schedule was changed'),
('audit_action_addon_047', 'plugins', 'plugin.kvm.snapshot_schedule.deleted', 'warning', 'Snapshot schedule was removed'),
('audit_action_addon_048', 'plugins', 'plugin.kvm.snapshot_schedule.run', 'info', 'Snapshot schedule was run on request'),
('audit_action_addon_049', 'plugins', 'plugin.kvm.snapshot_schedule.ran', 'success', 'Snapshot schedule fired on its own and took a snapshot'),
('audit_action_addon_050', 'plugins', 'plugin.kvm.snapshot_schedule.failed', 'error', 'Scheduled snapshot failed, so the retention window has a hole in it'),

('audit_action_addon_051', 'plugins', 'plugin.kvm.import.uploaded', 'success', 'Disk image was uploaded for import'),
('audit_action_addon_052', 'plugins', 'plugin.kvm.import.inspected', 'info', 'Uploaded disk image was inspected'),
('audit_action_addon_053', 'plugins', 'plugin.kvm.import.converted', 'info', 'Disk image conversion was started'),
('audit_action_addon_054', 'plugins', 'plugin.kvm.import.convert_finished', 'success', 'Disk image conversion finished and the volume is ready to attach'),
('audit_action_addon_055', 'plugins', 'plugin.kvm.import.convert_failed', 'error', 'Disk image conversion failed'),
('audit_action_addon_056', 'plugins', 'plugin.kvm.import.cancelled', 'warning', 'Disk image import was cancelled'),
('audit_action_addon_057', 'plugins', 'plugin.kvm.import.deleted', 'warning', 'Uploaded disk image was removed'),

('audit_action_addon_070', 'plugins', 'plugin.reporting.init', 'info', 'Reporting add-on started'),
('audit_action_addon_071', 'plugins', 'plugin.reporting.shutdown', 'info', 'Reporting add-on stopped'),
('audit_action_addon_072', 'plugins', 'plugin.reporting.report.generated', 'success', 'Report was generated'),
('audit_action_addon_073', 'plugins', 'plugin.reporting.report.exported', 'info', 'Report was exported to a file'),
('audit_action_addon_074', 'plugins', 'plugin.reporting.report.deleted', 'warning', 'Report was removed from the library'),
('audit_action_addon_075', 'plugins', 'plugin.reporting.settings.updated', 'info', 'Reporting settings were changed'),
('audit_action_addon_076', 'plugins', 'plugin.reporting.schedule.created', 'success', 'Report schedule was created'),
('audit_action_addon_077', 'plugins', 'plugin.reporting.schedule.updated', 'info', 'Report schedule was changed'),
('audit_action_addon_078', 'plugins', 'plugin.reporting.schedule.deleted', 'warning', 'Report schedule was removed'),
('audit_action_addon_079', 'plugins', 'plugin.reporting.schedule.run_requested', 'info', 'Report schedule was run on request'),
('audit_action_addon_080', 'plugins', 'plugin.reporting.schedule.ran', 'success', 'Report schedule fired on its own and delivered its report'),
('audit_action_addon_081', 'plugins', 'plugin.reporting.schedule.exported', 'info', 'A scheduled run''s report was exported to a file'),
('audit_action_addon_082', 'plugins', 'plugin.reporting.node.generate', 'info', 'Workflow node generated a report'),
('audit_action_addon_083', 'plugins', 'plugin.reporting.node.export', 'info', 'Workflow node exported a report'),
('audit_action_addon_084', 'plugins', 'plugin.reporting.node.schedule_run', 'info', 'Workflow node ran a report schedule'),

('audit_action_addon_090', 'plugins', 'plugin.kvm.vm.started', 'success', 'Virtual machine was started'),
('audit_action_addon_091', 'plugins', 'plugin.kvm.vm.stopped', 'warning', 'Virtual machine was powered off without a guest shutdown'),
('audit_action_addon_092', 'plugins', 'plugin.kvm.vm.paused', 'warning', 'Virtual machine vCPUs were paused'),
('audit_action_addon_093', 'plugins', 'plugin.kvm.vm.resumed', 'success', 'Virtual machine vCPUs were resumed'),
('audit_action_addon_094', 'plugins', 'plugin.compute.pool.updated', 'info', 'Storage pool capacity was re-sampled');
