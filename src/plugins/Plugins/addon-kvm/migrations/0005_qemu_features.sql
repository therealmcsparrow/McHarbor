-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE plugin_addon_kvm_vms ADD COLUMN run_state TEXT;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN guest_hostname TEXT;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN guest_os_name TEXT;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN guest_os_version TEXT;

ALTER TABLE plugin_addon_kvm_vms ADD COLUMN vcpus_max INTEGER;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN memory_current_mb INTEGER;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN virtio_mem INTEGER NOT NULL DEFAULT 0;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN hugepages INTEGER NOT NULL DEFAULT 0;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN numa_policy TEXT;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN numa_host_nodes TEXT;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN vsock_cid INTEGER;
ALTER TABLE plugin_addon_kvm_vms ADD COLUMN iothread INTEGER NOT NULL DEFAULT 0;

-- A context id must be unique per host: two guests sharing one is silent, and
-- the symptom is a channel that reaches the wrong guest. A partial index keeps
-- the constraint off the guests that have no CID at all.
CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vms_vsock
    ON plugin_addon_kvm_vms(environment_id, vsock_cid)
    WHERE vsock_cid IS NOT NULL;

ALTER TABLE plugin_addon_kvm_disks ADD COLUMN io_read_iops INTEGER;
ALTER TABLE plugin_addon_kvm_disks ADD COLUMN io_write_iops INTEGER;
ALTER TABLE plugin_addon_kvm_disks ADD COLUMN io_read_bps INTEGER;
ALTER TABLE plugin_addon_kvm_disks ADD COLUMN io_write_bps INTEGER;
ALTER TABLE plugin_addon_kvm_disks ADD COLUMN share_rw INTEGER NOT NULL DEFAULT 0;
ALTER TABLE plugin_addon_kvm_disks ADD COLUMN boot_index INTEGER;

ALTER TABLE plugin_addon_kvm_nics ADD COLUMN boot_index INTEGER;

-- Per-host settings this plugin owns. Today it holds only the virtiofs share
-- roots, which must be an operator decision per host: the default of no roots
-- refuses every share, because an unconfigured install that silently allowed
-- "/" would hand the host to any guest.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_host_settings (
    environment_id TEXT NOT NULL,
    key            TEXT NOT NULL,
    value          TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (environment_id, key)
);

-- One virtiofs share. tag is what the guest mounts by, so it is unique per
-- guest rather than per host.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_shares (
    id          TEXT PRIMARY KEY,
    vm_id       TEXT NOT NULL,
    tag         TEXT NOT NULL,
    host_path   TEXT NOT NULL,
    read_only   INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    FOREIGN KEY (vm_id) REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_addon_kvm_shares_tag
    ON plugin_addon_kvm_shares(vm_id, tag);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_shares_vm
    ON plugin_addon_kvm_shares(vm_id);

-- One incremental-backup chain: a persistent dirty bitmap on one disk, and the
-- runs taken against it. A chain is invalidated when its bitmap disappears,
-- which a qcow2 rebuild or an out-of-band qemu-img write both cause — and an
-- "incremental" taken against a lost bitmap restores to corruption, so the
-- invalidation has to be recorded rather than inferred.
CREATE TABLE IF NOT EXISTS plugin_addon_kvm_backup_chains (
    id           TEXT PRIMARY KEY,
    vm_id        TEXT NOT NULL,
    disk_id      TEXT NOT NULL,
    bitmap_name  TEXT NOT NULL,
    last_full_at TEXT,
    last_run_at  TEXT,
    invalidated  INTEGER NOT NULL DEFAULT 0,
    invalid_why  TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    FOREIGN KEY (vm_id) REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_addon_kvm_chains_disk
    ON plugin_addon_kvm_backup_chains(disk_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_chains_vm
    ON plugin_addon_kvm_backup_chains(vm_id);
