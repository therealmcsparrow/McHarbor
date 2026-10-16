# McHarbor Compute Foundation

Storage pools, networks, guest images and per-host capability checks for running
virtual machines and system containers on your own hosts — as an installable
McHarbor add-on.

Install it from **App Store → Plugins → Compute Foundation**, or let it arrive
on its own: installing **Virtual Machines** (`addon-kvm`) or **System
Containers** (`addon-lxc`) installs this alongside them, and removing the last
of the two removes it again.

## What it is for

The two guest plugins own guests. This one owns everything a guest needs to
exist before there is a guest:

| Area | What you get |
|---|---|
| Capability | What each host can actually run, checked dependency by dependency, with per-distro remediation text |
| Pools | Storage for guest disks and rootfs trees: directory, zfs, btrfs and lvm, with capacity sampling |
| Networks | Bridges on the host, created or adopted, with VLAN support |
| Images | Cloud images and rootfs archives, fetched and verified on the host |

Installed on its own it is still useful: it tells you, per host, what would be
needed to run guests there and how to install it.

## The capability panel

The part worth reading the code for. Every dependency floor is checked
separately — `/dev/kvm`, the qemu binaries and their machine types, cgroup v2 on
the unified hierarchy, idmapped mount support, `newuidmap`/`newgidmap`, seccomp,
the systemd version, the bridge tooling — and each check carries its own
remediation string built from the host's `/etc/os-release`.

A check that cannot say how to fix itself leaves the operator stuck, which is
the failure this panel exists to prevent. `CanRunVMs` and `CanRunCTs` are
separate answers: a host can be perfectly able to run containers and have no
`/dev/kvm` at all.

## Images never transit McHarbor

The agent fetches an image straight from the distribution mirror to the host's
pool. Only progress comes back over the transport. A checksum is mandatory and
verified while the bytes are written rather than afterwards, an OpenPGP
signature is verified where the distribution publishes one, and the file lands
under a `.partial-` name and is renamed atomically only once it verifies — so a
half-downloaded image is never a candidate to boot from.

## What it stores

Tables under `plugin_addon_compute_`: pools, networks and cached images.

The host restricts each plugin to its own table prefix, which has a consequence
worth knowing about when reading the two guest plugins: **they cannot read these
rows**. That is why a KVM disk carries its pool's backend and target alongside
the pool id, and a container's interface carries its bridge alongside the network
id. The denormalisation is deliberate — the agent acts on a target, not an id.

## Permissions

Three keys, declared in `plugin.yaml` under `adds_permissions`:

| Key | Preset | Grants |
|---|---|---|
| `compute.view` | viewer, editor, operator | See hosts, capability, pools, networks, images |
| `compute.manage` | editor, operator | Create pools and networks, fetch images |
| `compute.delete` | operator | Remove a pool, a network or a cached image |

Both guest plugins name `compute.view` under `consumes_permissions`, so a user
without it sees their pool and network pickers disabled with a tooltip naming
the key, rather than an empty picker that looks broken.

## Where the work happens

Not in this plugin. The McHarbor process cannot create a bridge on the host,
initialise a zfs pool, or open `/dev/kvm` — it is a container. So the executor
lives in the **remote agent** and is part of core, reached through
`host.AgentRPC` (`agent.rpc` scope token plus an `agent-rpc` capability).
The field is typed `any`; the plugin asserts it against its own interface.

`AgentRPC` is nil when no agent transport is wired. The pages then render and
say that no host can run guests, rather than the section refusing to load.

## Contributions

- **Workflow nodes** — `compute-host-capability`, `compute-pool-capacity`,
  `compute-ensure-image`. Infrastructure only: starting and stopping guests
  belongs to the plugins that own guests. What lives here is what a compute
  workflow needs to *decide* with — is this host able, does this pool have room,
  is this image present.
- **Dashboard widgets** — `compute-capacity`, `compute-hosts`.
- **Reports and alert conditions** — host readiness and image and lease hygiene reports, the inventory, capacity and health facets of the shared reports, and alert conditions for host capability.
- **i18n** — the `compute` namespace, six locales.

## Related

- `Plugins/addon-kvm` — KVM virtual machines.
- `Plugins/addon-lxc` — LXC system containers.
- `.documentation/plugins/type-compute.md` — the family, and the core/plugin
  split behind it.
