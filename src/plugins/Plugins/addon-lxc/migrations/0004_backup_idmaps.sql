-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_run_idmaps (
    id         TEXT PRIMARY KEY REFERENCES plugin_addon_lxc_backup_runs(id) ON DELETE CASCADE,
    privileged INTEGER NOT NULL DEFAULT 0,
    uid_start  INTEGER NOT NULL DEFAULT 0,
    uid_count  INTEGER NOT NULL DEFAULT 0,
    gid_start  INTEGER NOT NULL DEFAULT 0,
    gid_count  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
