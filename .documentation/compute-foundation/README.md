# Compute Foundation

The shared foundation under virtual machines and system containers: node capability probing, storage pools, guest networks and the image catalog. On its own it manages nothing.

- **Slug** `addon-compute`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** its own section in the sidebar, and its own pages under `/plugins/<slug>`
- **Gated by** the permission keys it declares (see below)

## How it works

- Its manifest carries `installed_as: dependency` and `required_by: addon-kvm, addon-lxc`. Installing either of those installs it; removing the last one removes it again. You do not install it by hand.
- It holds `agent.rpc` and an `agent-rpc` capability, which let it send the compute, pool, network and image messages to a host's agent. A plugin cannot extend `src/agent/protocol.go`, so the host keeps a ceiling of routable message types, and the plugin declares which of them it uses. See [Agent RPC](../development/contracts/agent-rpc/README.md).
- `compute.view` / `compute.manage` / `compute.delete` are environment-scoped and gate the shared surfaces: pools, networks, images, and the capability panel.
- The bundled catalog ships **49 images**: desktop, server and ARM64 Linux, plus Windows and
  Android. Server entries are cloud images wherever the project publishes one, because a cloud
  image boots in seconds and takes its user and SSH key from cloud-init; an installer ISO is the
  fallback only where no cloud image exists.
- Each entry shows its **distribution logo**, and the Images page carries an **operating-system
  filter** built from the catalog itself: add a JSON file for a distribution nobody has used
  before and it appears in the filter with no code change.
- An entry whose publisher offers no checksum McHarbor can verify carries **none**, rather than a
  weaker one recorded as though it were real. Such an image is labelled unverified and costs an
  explicit acknowledgement at download. MD5 is not accepted: it is no defence against a tampered
  mirror.
- It contributes two widgets, three workflow nodes, and the shared `compute` translation namespace.
- Images you supply yourself go into a shared directory McHarbor writes and every host mounts, rather than over the agent transport. See [Configuration → The shared image library](./configuration.md#the-shared-image-library).

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsFor` | Yes |  |

## What it contributes

| | |
|---|---|
| Routes | 43 |
| Sidebar entries | 4 |
| Dashboard widgets | `compute-capacity`, `compute-hosts`, `compute-images`, `compute-networks` |
| Workflow nodes | `compute-host-capability`, `compute-pool-capacity`, `compute-ensure-image` |
| Migrations | `0001_init.sql`, `0002_image_library.sql`, `0003_locations.sql`, `0004_direct_networks.sql`, `0005_image_cache_phase.sql` |
| Backend | 21 Go files, ~8,270 lines |
| Required by | `addon-kvm`, `addon-lxc` |

## Permissions it adds

| Key | Scope | What it allows |
|---|---|---|
| `compute.view` | env | View compute infrastructure |
| `compute.manage` | env | Manage compute infrastructure |
| `compute.images.upload` | env | Upload an image file into the shared image library |
| `compute.delete` | env | Delete compute infrastructure |

## In this section

| Page | What it covers |
|---|---|
In reading order: can I, what does the host need, then do it.

| Page | What it covers |
|---|---|
| [Supported hosts and guests](./supported-platforms.md) | What McHarbor and the agent run on, which host to recommend, and what a guest may be |
| [Host requirements](./host-requirements.md) | What a host needs to run guests, item by item, and why a containerised agent cannot |
| [Getting started](./getting-started.md) | Nothing to a running guest, in seven steps |
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [The host detail page](./host-detail.md) | One host's overview, settings, readiness tests and logs |
| [Guest networking](./guest-networking.md) | Bridged, external, managed and macvlan modes, converting a host that already runs Docker macvlan, and diagnosing a guest that gets no address |
| [Image catalog](./image-catalog.md) | The JSON files behind the downloadable image list, and how to add one |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../development/add-ons/compute-foundation.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Report catalog](../reporting/reports/README.md): the two compute reports this plugin contributes, and the facets it answers for the shared reports
- [Add-ons](../plugins/addons.md): what the six have in common, and what the plugin runtime does with them
- [Plugin catalog](../plugins/catalog.md): every shipped plugin
