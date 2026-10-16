-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- LXC system container schema.
--
-- Tables derive from the slug: the host restricts each plugin to its own
-- `plugin_<slug with hyphens as underscores>_` prefix, so `addon-lxc` writes
-- `plugin_addon_lxc_*`.
--
-- That restriction has the same design consequence it has in addon-kvm: this
-- plugin CANNOT read addon-compute's pool, network or image rows. So a
-- container carries its pool's backend and target alongside the pool id, and a
-- NIC carries its bridge alongside the network id. The denormalization is
-- deliberate — the agent acts on a target, not on an id, so those are the
-- values it needs anyway, and copying them at create time also means a
-- container keeps working if its pool row is later renamed.

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_containers (
    id             TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL,
    name           TEXT NOT NULL,
    description    TEXT,

    -- The agent is the only authority on this. The backend never writes a
    -- state it has not been told: "unknown" is a first-class value, entered
    -- whenever the owning agent is offline, because guessing a container is
    -- still running because it was ten minutes ago is how a management UI
    -- lies. "frozen" is a state of its own rather than a flag on running —
    -- a frozen container holds its memory and its pids but executes nothing,
    -- and collapsing that into "running" would be wrong in the one direction
    -- that matters.
    state TEXT NOT NULL DEFAULT 'defined',

    os_family  TEXT,
    os_version TEXT,

    -- Where the rootfs lives. pool_id names addon-compute's row;
    -- pool_backend and pool_target are the copy this plugin can act on.
    pool_id      TEXT NOT NULL,
    pool_backend TEXT NOT NULL,
    pool_target  TEXT NOT NULL,
    rootfs_path  TEXT NOT NULL,
    -- The imported, reference-counted rootfs a CoW pool clones from. Set,
    -- creation was a metadata operation and this container cannot outlive its
    -- template.
    template_path TEXT,

    -- systemd | openrc | none. systemd needs a delegated cgroup v2 subtree,
    -- tmpfs at /run and /tmp, a populated /dev, and SIGRTMIN+3 as its stop
    -- signal. Sending SIGTERM to systemd's PID 1 is what produces the
    -- "container takes 90 seconds to stop" complaint, so the profile is
    -- recorded per container rather than guessed at stop time.
    init_system TEXT NOT NULL DEFAULT 'systemd',
    init_argv   TEXT,

    -- Off by default and gated behind ct.privileged. Root inside a privileged
    -- container is root on the host in every way that matters.
    privileged INTEGER NOT NULL DEFAULT 0,
    -- Containers inside this container. Also privileged: it needs a relaxed
    -- device cgroup and an apparmor profile that permits mounts.
    nesting INTEGER NOT NULL DEFAULT 0,

    -- Id mapping for the unprivileged case. Ranges are allocated distinct and
    -- non-overlapping per container — overlapping ranges would let one
    -- container's root touch another's files, which defeats the point.
    uid_map_start INTEGER,
    uid_map_count INTEGER,
    gid_map_start INTEGER,
    gid_map_count INTEGER,
    -- idmapped | chown. An idmapped mount is instant and leaves the rootfs
    -- shareable; the chown fallback rewrites every inode and is one-way, which
    -- is worth recording because it decides whether a template can be reused.
    idmap_mechanism TEXT NOT NULL DEFAULT 'idmapped',

    -- Live cgroup v2 limits. Unlike a VM's memory, every one of these applies
    -- to a running container without a restart, which is why they are editable
    -- in place in the UI and why `applied_*` below exists.
    cpu_max           TEXT,
    cpu_weight        INTEGER,
    cpuset            TEXT,
    memory_max_bytes  INTEGER,
    memory_high_bytes INTEGER,
    swap_max_bytes    INTEGER NOT NULL DEFAULT 0,
    pids_max          INTEGER,
    io_max            TEXT,
    disk_quota_bytes  INTEGER,

    -- What the kernel actually accepted, as reported by the agent after
    -- writing the cgroup files. Stored separately from the requested values
    -- because the kernel rounds and clamps, and echoing the request back would
    -- be a lie the operator has no way to detect.
    applied_limits     TEXT,
    limits_applied_at  TEXT,

    autostart         INTEGER NOT NULL DEFAULT 0,
    autostart_order   INTEGER NOT NULL DEFAULT 0,
    autostart_delay_s INTEGER NOT NULL DEFAULT 0,

    is_template        INTEGER NOT NULL DEFAULT 0,
    template_source_id TEXT REFERENCES plugin_addon_lxc_containers(id) ON DELETE SET NULL,

    -- Read from the container's network namespace by the agent, not reported
    -- from inside the container. Display only, escaped at render, never used
    -- to build a link and never used for any authorization decision.
    observed_ips TEXT,

    unit_name TEXT,
    init_pid  INTEGER,

    last_started_at TEXT,
    exit_reason     TEXT,
    last_error      TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (environment_id, name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_containers_env
    ON plugin_addon_lxc_containers(environment_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_containers_state
    ON plugin_addon_lxc_containers(state);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_containers_template
    ON plugin_addon_lxc_containers(template_source_id);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_nics (
    id           TEXT PRIMARY KEY,
    container_id TEXT NOT NULL
                 REFERENCES plugin_addon_lxc_containers(id) ON DELETE CASCADE,

    network_id TEXT NOT NULL,
    bridge     TEXT NOT NULL,
    vlan_tag   INTEGER,

    slot       INTEGER NOT NULL,
    -- The name the interface takes inside the container. Distinct from the
    -- host end below, which is what an operator greps for in `ip link` on the
    -- host and can otherwise only find by process of elimination.
    guest_name TEXT NOT NULL DEFAULT 'eth0',
    mac        TEXT NOT NULL,
    veth_host  TEXT,

    address_mode TEXT NOT NULL DEFAULT 'dhcp',
    static_ip    TEXT,
    static_gw    TEXT,
    static_dns   TEXT,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (container_id, slot),
    UNIQUE (container_id, guest_name),
    -- Globally unique, not per-container. Two guests with the same MAC on one
    -- segment is a failure that presents as intermittent network loss and
    -- names nothing; the allocator retries on collision rather than allowing
    -- it.
    UNIQUE (mac)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_nics_ct
    ON plugin_addon_lxc_nics(container_id);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_mounts (
    id           TEXT PRIMARY KEY,
    container_id TEXT NOT NULL
                 REFERENCES plugin_addon_lxc_containers(id) ON DELETE CASCADE,

    -- bind | volume | device.
    --
    -- The distinction is a security boundary, not a convenience: a volume is a
    -- directory the plugin created inside a pool it already controls, while a
    -- bind is an arbitrary host path and is therefore a host-compromise vector
    -- (bind /etc and a container root rewrites the host's shadow file). Only
    -- volume takes ct.manage; bind and device take ct.privileged. The agent
    -- re-validates the source independently — the backend check is UX, the
    -- agent check is security.
    kind        TEXT NOT NULL,
    source      TEXT NOT NULL,
    destination TEXT NOT NULL,
    readonly    INTEGER NOT NULL DEFAULT 0,
    -- Whether the mount is id-shifted into the container's uid range. Without
    -- it, every file in an unprivileged container's bind mount appears owned
    -- by nobody, which is the single most common "why can't my container write
    -- to this" question.
    shift_ids INTEGER NOT NULL DEFAULT 1,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (container_id, destination)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_mounts_ct
    ON plugin_addon_lxc_mounts(container_id);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_snapshots (
    id           TEXT PRIMARY KEY,
    container_id TEXT NOT NULL
                 REFERENCES plugin_addon_lxc_containers(id) ON DELETE CASCADE,
    parent_id    TEXT REFERENCES plugin_addon_lxc_snapshots(id) ON DELETE SET NULL,

    name        TEXT NOT NULL,
    description TEXT,
    -- zfs | btrfs | lvm | rsync. A property of the pool backend, recorded
    -- because reverting has to use the same mechanism and a pool can be
    -- replaced under a container. A dir pool has no snapshot mechanism of its
    -- own, so it gets an rsync copy — correct, much slower, and labelled as
    -- such rather than quietly pretending to be a snapshot.
    mechanism TEXT NOT NULL,
    -- Whether the container was frozen for the duration. A snapshot of a
    -- running, unfrozen container is crash-consistent — the equivalent of
    -- pulling the power cord — and the operator should know which one they
    -- have before they rely on it.
    was_frozen INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER,
    -- Depth in the chain, stored, shown and capped for the same reason it is
    -- in addon-kvm: deep CoW chains destroy read performance.
    chain_depth INTEGER NOT NULL DEFAULT 0,
    -- The backend-native handle used to revert or delete.
    handle TEXT NOT NULL,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (container_id, name)
);

CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_snapshots_ct
    ON plugin_addon_lxc_snapshots(container_id);
CREATE INDEX IF NOT EXISTS idx_plugin_addon_lxc_snapshots_parent
    ON plugin_addon_lxc_snapshots(parent_id);

CREATE TABLE IF NOT EXISTS plugin_addon_lxc_provisioning (
    container_id TEXT PRIMARY KEY
                 REFERENCES plugin_addon_lxc_containers(id) ON DELETE CASCADE,

    hostname TEXT,
    timezone TEXT,
    ssh_keys TEXT,
    -- A SHA-512 crypt(3) hash, computed on the backend before it ever reaches
    -- the rootfs. The plaintext exists only for the duration of the create
    -- request and is never stored here — the agent writes the hash into
    -- /etc/shadow directly, which is the same place it would have ended up.
    password_hash TEXT,

    -- systemd-networkd | interfaces | none. A container has no cloud-init and
    -- no DHCP client guarantee, so static addressing means writing the
    -- distro's own config file. Getting this wrong leaves a container with no
    -- network and no console route to fix it, which is why the format is
    -- chosen from the detected OS family rather than assumed.
    net_config_format TEXT NOT NULL DEFAULT 'systemd-networkd',

    -- Applied once, on first start. Recorded so a restart does not re-run
    -- provisioning and undo whatever the operator has since changed inside the
    -- container.
    applied_at TEXT,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
