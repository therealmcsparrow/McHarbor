-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_imports (
    id          TEXT PRIMARY KEY,
    file_name   TEXT NOT NULL,
    source_kind TEXT NOT NULL DEFAULT '',
    staged_path TEXT NOT NULL,
    size_bytes  INTEGER NOT NULL DEFAULT 0,

    state      TEXT NOT NULL,
    progress   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,

    environment_id TEXT,

    inspection TEXT,

    vm_id TEXT,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_imports_state
    ON plugin_addon_kvm_imports(state);
