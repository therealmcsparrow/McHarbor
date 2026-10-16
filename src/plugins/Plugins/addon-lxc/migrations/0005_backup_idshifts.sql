-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_backup_run_idshifts (
    id         TEXT PRIMARY KEY REFERENCES plugin_addon_lxc_backup_runs(id) ON DELETE CASCADE,
    id_shift   TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
