-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_compute_locations (
    id          TEXT PRIMARY KEY,
    scope_id    TEXT NOT NULL UNIQUE,
    vm_path     TEXT,
    disk_path   TEXT,
    image_path  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_locations_scope
    ON plugin_addon_compute_locations(scope_id);
