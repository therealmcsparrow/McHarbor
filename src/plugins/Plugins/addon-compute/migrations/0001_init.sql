-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Compute foundation schema.
--
-- These tables were specified in the roadmap as core migrations 103–105. They
-- are plugin tables instead, under this plugin's own `plugin_addon_compute_` prefix,
-- because the whole compute capability ships as plugins — an installation that
-- runs no guests carries none of this schema, exactly as it carries none of
-- Kubernetes' since that moved out of core.
--
-- One consequence worth stating: the roadmap put five compute columns on the
-- core `environments` table. A plugin must not alter a host table, so that
-- state lives in `plugin_addon_compute_environments` below, keyed by the environment
-- id. Deleting an environment leaves an orphan row here rather than cascading,
-- which the service prunes on read; the alternative — a foreign key into a host
-- table — would make this plugin impossible to uninstall cleanly.

-- Table names derive from the slug: the host restricts every plugin to its own
-- `plugin_<slug with hyphens as underscores>_` prefix, so `addon-compute`
-- writes `plugin_addon_compute_*`. That restriction has a design consequence
-- worth knowing about: addon-kvm and addon-lxc CANNOT read these tables.
--
-- Which is why a guest's disk row carries its pool's backend and target
-- alongside the pool id, and a NIC row carries its bridge alongside the network
-- id. The denormalization is deliberate. The alternative would be a
-- cross-plugin read the host correctly refuses, and the copied values are
-- exactly what the agent needs anyway — it acts on a target, not on an id.

-- Per-environment compute settings.
CREATE TABLE IF NOT EXISTS plugin_addon_compute_environments (
    id                 TEXT PRIMARY KEY,
    environment_id     TEXT NOT NULL UNIQUE,
    -- Only settable on environments whose connection_type is `agent`. The
    -- McHarbor container cannot host guests: a `docker compose up -d` would
    -- kill every running VM, and taps created in its network namespace would
    -- sit behind Docker's NAT instead of on the LAN.
    compute_enabled    INTEGER NOT NULL DEFAULT 0,
    -- The last capability probe, stored whole as JSON. It is a snapshot of
    -- someone else's machine and its shape follows the agent's, so normalizing
    -- it into columns would mean a migration every time a check is added.
    capabilities       TEXT,
    probed_at          TEXT,
    default_pool_id    TEXT,
    default_network_id TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_env_enabled
    ON plugin_addon_compute_environments(compute_enabled);

-- Storage pools: a named place on one host where guest volumes live.
CREATE TABLE IF NOT EXISTS plugin_addon_compute_pools (
    id             TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL,
    name           TEXT NOT NULL,
    backend        TEXT NOT NULL,          -- dir | zfs | lvm | btrfs
    target         TEXT NOT NULL,          -- path, dataset, vg/thinpool, subvolume
    is_default     INTEGER NOT NULL DEFAULT 0,
    -- Capacity is sampled on the existing host-stats interval rather than by a
    -- second timer, so these are a recent snapshot and not live values.
    capacity_bytes  INTEGER,
    used_bytes      INTEGER,
    allocated_bytes INTEGER,
    volume_count    INTEGER NOT NULL DEFAULT 0,
    sampled_at      TEXT,
    state           TEXT NOT NULL DEFAULT 'unknown',
    last_error      TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (environment_id, name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_pools_env
    ON plugin_addon_compute_pools(environment_id);

-- Guest networks. Both guest kinds share these rows, which is a real
-- simplification for the operator: one network model, not one per guest type.
CREATE TABLE IF NOT EXISTS plugin_addon_compute_networks (
    id               TEXT PRIMARY KEY,
    environment_id   TEXT NOT NULL,
    name             TEXT NOT NULL,
    mode             TEXT NOT NULL,        -- bridged | managed
    bridge           TEXT NOT NULL,
    vlan_tag         INTEGER,
    -- Managed mode only. Bridged networks get their addressing from the LAN's
    -- own DHCP server and McHarbor creates nothing beyond the tap or veth.
    subnet_cidr      TEXT,
    gateway_ip       TEXT,
    dhcp_enabled     INTEGER NOT NULL DEFAULT 0,
    dhcp_range_start TEXT,
    dhcp_range_end   TEXT,
    nat_enabled      INTEGER NOT NULL DEFAULT 0,
    uplink_link      TEXT,
    is_default       INTEGER NOT NULL DEFAULT 0,
    state            TEXT NOT NULL DEFAULT 'unknown',
    last_error       TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    UNIQUE (environment_id, name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_networks_env
    ON plugin_addon_compute_networks(environment_id);

-- DHCP leases on managed networks.
CREATE TABLE IF NOT EXISTS plugin_addon_compute_leases (
    id         TEXT PRIMARY KEY,
    network_id TEXT NOT NULL REFERENCES plugin_addon_compute_networks(id) ON DELETE CASCADE,
    mac        TEXT NOT NULL,
    ip         TEXT NOT NULL,
    -- Guest-supplied and therefore untrusted: display only, escaped at render,
    -- never used to build a link or a command.
    hostname   TEXT,
    guest_kind TEXT,
    guest_id   TEXT,
    expires_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (network_id, mac)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_leases_network
    ON plugin_addon_compute_leases(network_id);

-- The image catalog: one table for all three artifact kinds.
CREATE TABLE IF NOT EXISTS plugin_addon_compute_images (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,          -- cloud_image | rootfs | iso
    distro        TEXT NOT NULL,
    release       TEXT NOT NULL,
    arch          TEXT NOT NULL,
    variant       TEXT,
    display_name  TEXT NOT NULL,
    source_url    TEXT NOT NULL,
    checksum_url  TEXT,
    checksum      TEXT,
    checksum_algo TEXT NOT NULL DEFAULT 'sha256',
    signature_url TEXT,
    size_bytes    INTEGER,
    -- A built-in entry's checksum verification is mandatory and cannot be
    -- overridden. Only an operator-supplied URL may waive it, and the waiver
    -- is recorded in the audit row.
    is_builtin    INTEGER NOT NULL DEFAULT 0,
    -- init_system tells the container path which profile a rootfs needs, so
    -- the right one is chosen automatically instead of asked about.
    init_system   TEXT,
    -- net_config_format is the per-distro network config writer to use for a
    -- static address.
    net_config_format TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_images_kind
    ON plugin_addon_compute_images(kind);

-- Per-environment download state for a catalog entry.
CREATE TABLE IF NOT EXISTS plugin_addon_compute_image_cache (
    id             TEXT PRIMARY KEY,
    image_id       TEXT NOT NULL REFERENCES plugin_addon_compute_images(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL,
    local_path     TEXT NOT NULL,
    size_bytes     INTEGER,
    verified       INTEGER NOT NULL DEFAULT 0,
    signed         INTEGER NOT NULL DEFAULT 0,
    state          TEXT NOT NULL,          -- pending | downloading | ready | failed
    progress_pct   INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT,
    last_used_at   TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    UNIQUE (image_id, environment_id)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_image_cache_env
    ON plugin_addon_compute_image_cache(environment_id);

-- The built-in catalog.
--
-- Every entry points at the distribution's own mirror. Notably absent is
-- images.linuxcontainers.org, which would be the obvious rootfs source and is
-- deliberately not used: it is a competing project's infrastructure, served
-- over a protocol that project defines, and building the container create
-- dialog on it would mean McHarbor stops working the day that service adds
-- authentication or rate limits.
--
-- Checksum URLs rather than literal checksums, because a point release moves
-- the artifact and a pinned hash would go stale silently. The agent fetches the
-- published checksum file, verifies its signature where one exists, and matches
-- this image's filename inside it.
INSERT OR IGNORE INTO plugin_addon_compute_images
    (id, kind, distro, release, arch, display_name, source_url, checksum_url,
     checksum_algo, is_builtin, init_system, net_config_format, created_at, updated_at)
VALUES
    ('img-ubuntu-2404-amd64', 'cloud_image', 'ubuntu', '24.04', 'x86_64',
     'Ubuntu 24.04 LTS (Noble)',
     'https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img',
     'https://cloud-images.ubuntu.com/noble/current/SHA256SUMS',
     'sha256', 1, 'systemd', 'systemd-networkd',
     datetime('now'), datetime('now')),

    ('img-ubuntu-2204-amd64', 'cloud_image', 'ubuntu', '22.04', 'x86_64',
     'Ubuntu 22.04 LTS (Jammy)',
     'https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img',
     'https://cloud-images.ubuntu.com/jammy/current/SHA256SUMS',
     'sha256', 1, 'systemd', 'systemd-networkd',
     datetime('now'), datetime('now')),

    ('img-debian-13-amd64', 'cloud_image', 'debian', '13', 'x86_64',
     'Debian 13 (Trixie)',
     'https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2',
     'https://cloud.debian.org/images/cloud/trixie/latest/SHA512SUMS',
     'sha512', 1, 'systemd', 'interfaces',
     datetime('now'), datetime('now')),

    ('img-debian-12-amd64', 'cloud_image', 'debian', '12', 'x86_64',
     'Debian 12 (Bookworm)',
     'https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2',
     'https://cloud.debian.org/images/cloud/bookworm/latest/SHA512SUMS',
     'sha512', 1, 'systemd', 'interfaces',
     datetime('now'), datetime('now')),

    ('img-fedora-41-amd64', 'cloud_image', 'fedora', '41', 'x86_64',
     'Fedora Cloud 41',
     'https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2',
     'https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-41-1.4-x86_64-CHECKSUM',
     'sha256', 1, 'systemd', 'systemd-networkd',
     datetime('now'), datetime('now')),

    -- Alpine is the smallest useful rootfs and boots in well under a second on
    -- a copy-on-write pool. It runs OpenRC rather than systemd, which takes
    -- the lighter container profile — recorded here so the right one is chosen
    -- automatically.
    ('img-alpine-321-rootfs-amd64', 'rootfs', 'alpine', '3.21', 'x86_64',
     'Alpine Linux 3.21 (minimal rootfs)',
     'https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz',
     'https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz.sha256',
     'sha256', 1, 'openrc', 'interfaces',
     datetime('now'), datetime('now')),

    ('img-ubuntu-2404-rootfs-amd64', 'rootfs', 'ubuntu', '24.04', 'x86_64',
     'Ubuntu 24.04 Base (rootfs)',
     'https://cdimage.ubuntu.com/ubuntu-base/releases/24.04/release/ubuntu-base-24.04.1-base-amd64.tar.gz',
     'https://cdimage.ubuntu.com/ubuntu-base/releases/24.04/release/SHA256SUMS',
     'sha256', 1, 'systemd', 'systemd-networkd',
     datetime('now'), datetime('now'));
