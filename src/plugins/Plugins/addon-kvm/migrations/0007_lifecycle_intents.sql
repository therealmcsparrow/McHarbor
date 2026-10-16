-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_lifecycle_intents (
    id             TEXT PRIMARY KEY,
    vm_id          TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    action         TEXT NOT NULL,
    actor          TEXT NOT NULL,
    source         TEXT NOT NULL,
    outcome        TEXT NOT NULL DEFAULT 'pending',
    error          TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_lifecycle_intents_vm
    ON plugin_addon_kvm_lifecycle_intents(vm_id, created_at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_lifecycle_intents_created
    ON plugin_addon_kvm_lifecycle_intents(created_at);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_vm_events (
    id             TEXT PRIMARY KEY,
    vm_id          TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    kind           TEXT NOT NULL,
    reason         TEXT,
    device         TEXT,
    at             TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_events_vm
    ON plugin_addon_kvm_vm_events(vm_id, at);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_events_created
    ON plugin_addon_kvm_vm_events(created_at);
