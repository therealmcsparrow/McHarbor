-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_vm_metrics (
    id               TEXT PRIMARY KEY,
    vm_id            TEXT NOT NULL,
    environment_id   TEXT NOT NULL,
    sampled_at       TEXT NOT NULL,
    cpu_percent      REAL NOT NULL DEFAULT 0,
    memory_bytes     INTEGER NOT NULL DEFAULT 0,
    memory_max_bytes INTEGER NOT NULL DEFAULT 0,
    disk_read_bytes  INTEGER NOT NULL DEFAULT 0,
    disk_write_bytes INTEGER NOT NULL DEFAULT 0,
    net_rx_bytes     INTEGER NOT NULL DEFAULT 0,
    net_tx_bytes     INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_metrics_vm_time
    ON plugin_addon_kvm_vm_metrics(vm_id, sampled_at);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_metrics_env
    ON plugin_addon_kvm_vm_metrics(environment_id);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_metrics_time
    ON plugin_addon_kvm_vm_metrics(sampled_at);
