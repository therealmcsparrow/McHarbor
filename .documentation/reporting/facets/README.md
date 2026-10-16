# Reporting: Cross-platform facets

What runs on an estate used to exist once per platform: a container inventory, a VM inventory,
a system container inventory, and so on, each with its own columns and its own idea of
"running". A **facet** is one question asked of every platform in one fixed row shape, so the
shared reports print one table with a **Platform** column instead of five documents to add up.

## The six facets

| Facet | Question | Columns beyond the shared identity |
|---|---|---|
| `inventory` | What runs, where, on what? | State, image (or OS or template), CPU cores, memory, disk, created |
| `capacity` | What is allocated against what exists, per host, pool or node? | Resource, allocated, used, capacity, unit |
| `protection` | Is it recoverable, and how recently? | Protected, method (snapshot, backup, replica, none), last good copy, off-host, RPO target, note |
| `health` | Is it up, degraded or down, and does it flap? | Status, restarts, flaps, since, detail |
| `exposure` | What is reachable, on which port, from where? | Protocol, port, target port, scope (public, internal, cluster), via |
| `cost` | Which resource-hours does it consume? | CPU core-hours, memory GB-hours, disk GB-hours, basis |

Every row also carries the platform token (`docker`, `kvm`, `lxc`, `kubernetes`, `swarm`,
`compute`), the object kind, id and name, the environment, the host, and an owner (a stack, a
namespace or a pool, whatever groups the object for its operator). The set is closed: the host
refuses any other facet name.

**Cost** is `allocated` for every platform today: what was reserved over the period, not what
was sampled. A report never adds allocated and measured figures without saying which is which.

## Who answers which facet

Docker is answered by the Reporting plugin itself. Each platform plugin answers for its own
objects, and only while it is installed, enabled and loaded.

| Platform | inventory | capacity | protection | health | exposure | cost |
|---|---|---|---|---|---|---|
| Docker (`docker`, Reporting plugin) | containers | Docker hosts | container backups | containers | published ports | containers |
| Virtual machines (`kvm`) | VMs | per-host allocation | snapshots and backups | guest state | not answered | allocated hours |
| System containers (`lxc`) | containers | per-host allocation | snapshots and backups | container state | not answered | allocated hours |
| Compute (`compute`) | hosts, pools, images | pools and hosts | not answered | host and agent health | not answered | not answered |
| Kubernetes (`kubernetes`) | deployments, pods | nodes | not answered | pods, deployments, nodes | services, ingresses | requests-based hours |
| Swarm (`swarm`) | services | nodes | not answered | nodes, services, quorum passage | published ports, overlay passage | reservation-based hours |

A platform that cannot answer a facet honestly does not declare it. Virtual machines and system
containers model no port forwards, Kubernetes and Swarm have no backup model, and Compute has
none of the workload-level facets beyond inventory, capacity and health, so those cells stay
empty rather than being invented.

Some platforms add a **passage** under their own sub-heading for what a row cannot say, for
example Swarm's manager quorum and overlay-network digests, which replaced the Swarm-specific
blocks the Reporting plugin used to build itself.

## Which reports show them

A facet appears as an **Across platforms** section in the report, placed after the last section
of its level. It has a per-platform breakdown, one table with a Platform column (capped at 150
rows; the breakdown above it is always complete), and each platform's own passage.

| Report | Facets shown |
|---|---|
| `executive-overview` | One KPI per facet, under "The estate across platforms" |
| `platform-inventory` | inventory, capacity, and the platform passages of health |
| `workload-inventory` | inventory |
| `environment-health` | health |
| `availability-slo`, `uptime-availability`, `stability-flapping` | health |
| `operational-readiness` | health, protection |
| `capacity-utilisation`, `resource-efficiency` | capacity |
| `environment-comparison` | inventory and capacity, as a per-environment roll-up |
| `backup-recovery`, `data-protection-rpo` | protection |
| `network-connectivity` | exposure |
| `security-compliance` | exposure, public scope only |
| `cost-allocation` | cost |
| `ad-hoc` | Every installed (facet, platform) pair is offered to the planner as a source |

Docker rows are not printed twice: where a report already lists containers in its own sections,
the Across platforms table leaves the Docker rows out and keeps the other platforms.

The reports without a facet are the identity and governance reports, the McHarbor-instance
reports (`instance-dr-readiness`, `ha-posture`, `agent-fleet`), and the reports whose subject is
Docker by definition, such as `image-supply-chain` and `volume-data-estate`.

### In the ad-hoc studio

Each (facet, platform) pair is its own source named `facet.<facet>.<platform>`, for example
`facet.protection.kvm`. The planner can therefore ask for "every VM and container without a
backup" and name exactly the platforms it needs. The projection carries the identity columns
(platform, kind, name, environment, owner) and the facet's own columns; host names and addresses
are dropped. The planner also has two Docker sources that were previously stubs: alert events
(`/api/alerts/events`) and auto-heal restarts (`/api/audit?action=autoheal`).

## What the reader sees when a platform is missing

- A plugin that is not installed declares nothing, so its platform is absent. The section names
  who answered ("Platforms: Docker, KVM, Kubernetes"), so a table with no KVM rows reads as
  "KVM is not installed" and not "KVM runs nothing".
- A plugin that fails becomes a source marked not available in the Methodology annex, with a
  fixed message. The provider's own error text is never printed, because it can carry a host
  name or a path. A failure never fails the document.
- A caller without the platform's view key gets no rows from that platform and a "not
  permitted" source, so the annex says the platform was withheld rather than empty. Each
  provider re-checks its own key for the generating user: `vm.view` (plus `vm.backup.view` for
  protection), `ct.view`, `compute.view`, `kubernetes.view`, and the `swarm_*.view` keys for the
  listings it reads.
- A report narrowed to one object covers Docker only, and says so. So does an installation with
  no platform plugin that contributes the facet.

The annex ends with a **Platform coverage** table: one row per facet, naming the platforms that
answered and any note.

## Report types that were removed

Twelve platform-specific report types are gone. They duplicated the shared reports with their
own columns. Their keys are deleted outright, with no alias.

| Removed key | Plugin | Replaced by |
|---|---|---|
| `kvm-inventory` | `addon-kvm` | `workload-inventory` |
| `lxc-inventory` | `addon-lxc` | `workload-inventory` |
| `compute-inventory` | `addon-compute` | `workload-inventory` |
| `kubernetes-workload-inventory` | `addon-kubernetes` | `workload-inventory` |
| `swarm-service-inventory` | `addon-swarm` | `workload-inventory` |
| `kvm-protection` | `addon-kvm` | `data-protection-rpo` |
| `lxc-protection` | `addon-lxc` | `data-protection-rpo` |
| `compute-capacity` | `addon-compute` | `capacity-utilisation` |
| `lxc-utilisation` | `addon-lxc` | `capacity-utilisation` |
| `kvm-allocation` | `addon-kvm` | `capacity-utilisation` |
| `kubernetes-cluster-health` | `addon-kubernetes` | `environment-health` |
| `swarm-cluster-health` | `addon-swarm` | `environment-health` |

`compute-capacity` is also the key of a Compute dashboard widget. The widget is unchanged.

`container-inventory` was renamed `workload-inventory` ("Workload Inventory"), because it now
lists every platform's workloads.

The Reporting plugin's migration `0007_facet_report_keys.sql` runs once and does the following:

- **Schedules** that named a removed key are repointed at the replacing report. The subject is
  cleared, because the replacing reports are estate-wide, so the schedule now produces the
  environment-wide document.
- **Schedules** named `container-inventory` become `workload-inventory`.
- **Stored documents** of `container-inventory` are relabelled `workload-inventory`, including
  the copy inside each schedule run.
- **Stored documents** of a removed type are marked `retired`. They stay in the library exactly
  as generated, with a **Report type retired** pill, and can still be opened and exported. They
  cannot be regenerated.

## Permissions

Generating any of these reports needs `reporting.generate`. What a facet shows is bounded by the
generating user's own view keys, as described above.

## Related

- [Report catalog](../reports/README.md)
- [Reporting](../README.md)
- [Contribute facets from a plugin](../../how-to/reporting/facets/README.md)
