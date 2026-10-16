-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- KVM virtual machine schema.
--
-- Tables derive from the slug: the host restricts each plugin to its own
-- `plugin_<slug with hyphens as underscores>_` prefix, so `addon-kvm` writes
-- `plugin_addon_kvm_*`.
--
-- That restriction has a design consequence visible throughout this file: this
-- plugin CANNOT read addon-compute's pool, network or image rows. So a disk
-- carries its pool's backend and target alongside the pool id, and a NIC
-- carries its bridge alongside the network id. The denormalization is
-- deliberate — the agent acts on a target, not on an id, so those are the
-- values it needs anyway, and copying them at create time also means a guest
-- keeps working if its pool row is later renamed.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_vms (
    id             TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL,
    name           TEXT NOT NULL,
    description    TEXT,

    -- The agent is the only authority on this. The backend never writes a
    -- state it has not been told: "unknown" is a first-class value, entered
    -- whenever the owning agent is offline, because guessing a VM is still
    -- running because it was ten minutes ago is how a management UI lies.
    state          TEXT NOT NULL DEFAULT 'defined',

    guest_os_family  TEXT,
    guest_os_variant TEXT,

    vcpus       INTEGER NOT NULL,
    cpu_model   TEXT NOT NULL DEFAULT 'host',
    cpu_sockets INTEGER NOT NULL DEFAULT 1,
    cpu_cores   INTEGER NOT NULL DEFAULT 1,
    cpu_threads INTEGER NOT NULL DEFAULT 1,
    memory_mb     INTEGER NOT NULL,
    memory_max_mb INTEGER,

    -- Pinned at creation as a concrete version (pc-q35-8.2, not q35) and never
    -- silently changed. An unpinned machine type is how guests break on a host
    -- package update; the UI offers an explicit, warned upgrade instead.
    machine_type TEXT NOT NULL,
    firmware     TEXT NOT NULL DEFAULT 'uefi',
    secure_boot  INTEGER NOT NULL DEFAULT 0,
    tpm_enabled  INTEGER NOT NULL DEFAULT 0,
    display_type TEXT NOT NULL DEFAULT 'virtio-gpu',

    autostart         INTEGER NOT NULL DEFAULT 0,
    autostart_order   INTEGER NOT NULL DEFAULT 0,
    autostart_delay_s INTEGER NOT NULL DEFAULT 0,

    is_template        INTEGER NOT NULL DEFAULT 0,
    template_source_id TEXT REFERENCES plugin_addon_kvm_vms(id) ON DELETE SET NULL,

    guest_agent_state TEXT,
    -- Guest-reported and untrusted: display only, escaped at render, never
    -- used to build a link and never used for any authorization decision.
    guest_reported_ips TEXT,

    -- An SMBIOS machine UUID. xid is not a UUID, so this is generated
    -- separately and regenerated on clone — a cloned guest that kept its
    -- machine UUID would be indistinguishable from its template to anything
    -- keying on it, licensing included.
    uuid      TEXT NOT NULL,
    unit_name TEXT,

    last_started_at TEXT,
    last_error      TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (environment_id, name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vms_env
    ON plugin_addon_kvm_vms(environment_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vms_state
    ON plugin_addon_kvm_vms(state);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vms_template
    ON plugin_addon_kvm_vms(template_source_id);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_disks (
    id    TEXT PRIMARY KEY,
    vm_id TEXT NOT NULL REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE,

    -- pool_id names addon-compute's row; pool_backend and pool_target are the
    -- copy this plugin can actually act on. See the header.
    pool_id      TEXT NOT NULL,
    pool_backend TEXT NOT NULL,
    pool_target  TEXT NOT NULL,

    slot        INTEGER NOT NULL,
    bus         TEXT NOT NULL DEFAULT 'virtio-blk',
    volume_path TEXT NOT NULL,
    format      TEXT NOT NULL DEFAULT 'qcow2',
    size_bytes  INTEGER NOT NULL,
    -- The cached cloud image or template disk a linked clone copies on write
    -- from. Set means this disk cannot outlive its backing file.
    backing_path TEXT,
    cache_mode   TEXT NOT NULL DEFAULT 'none',
    -- On by default, so a delete inside the guest actually returns capacity to
    -- a thin pool rather than only being marked free.
    discard  INTEGER NOT NULL DEFAULT 1,
    readonly INTEGER NOT NULL DEFAULT 0,
    is_boot  INTEGER NOT NULL DEFAULT 0,
    is_cdrom INTEGER NOT NULL DEFAULT 0,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (vm_id, slot)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_disks_vm
    ON plugin_addon_kvm_disks(vm_id);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_nics (
    id    TEXT PRIMARY KEY,
    vm_id TEXT NOT NULL REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE,

    network_id TEXT NOT NULL,
    bridge     TEXT NOT NULL,
    vlan_tag   INTEGER,

    slot     INTEGER NOT NULL,
    model    TEXT NOT NULL DEFAULT 'virtio-net',
    mac      TEXT NOT NULL,
    tap_name TEXT,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (vm_id, slot),
    -- Globally unique, not per-VM. Two guests with the same MAC on one segment
    -- is a failure that presents as intermittent network loss and names
    -- nothing; the allocator retries on collision rather than allowing it.
    UNIQUE (mac)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_nics_vm
    ON plugin_addon_kvm_nics(vm_id);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_snapshots (
    id        TEXT PRIMARY KEY,
    vm_id     TEXT NOT NULL REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE,
    parent_id TEXT REFERENCES plugin_addon_kvm_snapshots(id) ON DELETE SET NULL,

    name        TEXT NOT NULL,
    description TEXT,
    -- The mechanism that took it, which is a property of the pool backend:
    -- zfs | btrfs | lvm | qcow2. Recorded because reverting has to use the
    -- same one, and a pool can be replaced under a guest.
    mechanism TEXT NOT NULL,
    -- crash | filesystem | application. Surfaced honestly in the UI: without a
    -- guest agent a snapshot is crash-consistent, which is the equivalent of
    -- pulling the power cord, and the operator should know that.
    consistency TEXT NOT NULL DEFAULT 'crash',
    with_memory INTEGER NOT NULL DEFAULT 0,
    size_bytes  INTEGER,
    -- Depth in the chain. Deep external-qcow2 chains destroy read performance
    -- and are the classic way a homelab VM becomes mysteriously slow, so it is
    -- stored, shown, and capped.
    chain_depth INTEGER NOT NULL DEFAULT 0,
    -- The backend-native handle used to revert or delete.
    handle TEXT NOT NULL,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (vm_id, name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshots_vm
    ON plugin_addon_kvm_snapshots(vm_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshots_parent
    ON plugin_addon_kvm_snapshots(parent_id);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_cloud_init (
    vm_id TEXT PRIMARY KEY REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE,

    hostname     TEXT,
    timezone     TEXT,
    ssh_keys     TEXT,
    default_user TEXT,
    -- A SHA-512 crypt(3) hash, computed on the backend before it ever reaches
    -- the seed image. The plaintext exists only for the duration of the create
    -- request and is never stored here or written to the seed — the seed is
    -- readable by anything that can read the pool.
    password_hash TEXT,

    network_mode   TEXT NOT NULL DEFAULT 'dhcp',
    static_ip      TEXT,
    static_gateway TEXT,
    static_dns     TEXT,

    user_data_extra TEXT,
    -- Regenerated on clone, which is what makes cloud-init re-run first-boot
    -- setup instead of the clone inheriting the template's hostname and SSH
    -- host keys — the classic clone footgun.
    instance_id TEXT NOT NULL,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_devices (
    id    TEXT PRIMARY KEY,
    vm_id TEXT NOT NULL REFERENCES plugin_addon_kvm_vms(id) ON DELETE CASCADE,

    -- pci | usb | block. Every one of these can compromise the host rather
    -- than only the guest, which is why they sit behind vm.privileged.
    kind         TEXT NOT NULL,
    host_address TEXT NOT NULL,
    description  TEXT,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_devices_vm
    ON plugin_addon_kvm_devices(vm_id);
