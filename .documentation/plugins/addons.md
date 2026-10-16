# Add-ons

**Each add-on has its own section at the top level of this documentation**, because what
it adds is a product area and not an integration. This page is the plugin section's view
of them: what they have in common as plugins, and where their sections are.

An add-on owns a capability end to end: its own API routes, its own pages under
`/plugins/<slug>`, usually its own tables, and often dashboard widgets and workflow nodes.
Adapter plugins fill a slot the host defines; these define their own.

| Plugin | Slug | Routes | What it adds |
|---|---|---|---|
| [Compute Foundation](../compute-foundation/README.md) | `addon-compute` | 20 | The shared foundation for running virtual machines and system containers on your own hosts: node capability probing, storage pools, guest networks and the image catalog. |
| [Docker Swarm](../docker-swarm/README.md) | `addon-swarm` | 45 | Manage Docker Swarm from McHarbor: services, tasks, nodes, secrets, configs, swarm stacks and overlay networks, with cluster init, join, leave, join-token and unlock-key lifecycle, service scaling, rolling restarts and rollback. |
| [Kubernetes](../kubernetes/README.md) | `addon-kubernetes` | 13 | Manage Kubernetes clusters from McHarbor: pods, deployments and other workloads, services and networking, storage, config, namespaces, nodes and events, with pod logs, deployment scaling and rolling restarts. |
| [Reporting](../reporting/README.md) | `addon-reporting` | 22 | Generate board-presentable reports on environments, workloads, incidents, security, capacity, delivery and recovery. |
| [System Containers](../system-containers/README.md) | `addon-lxc` | 27 | Run full-system LXC containers on your own hosts: a whole distribution with its own init, users and services, in a fraction of a VM's memory and starting in about a second. |
| [Virtual Machines](../virtual-machines/README.md) | `addon-kvm` | 28 | Run KVM virtual machines on your own hosts. Create from a cloud image and reach a login prompt in the browser in under two minutes, or install from an ISO. |

Each add-on has a section of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering what it registers and how to build,
sign and fork it.

## What they have in common

- They declare every route in `plugin.yaml` with its permission key and, for mutations,
  its audit action. The host mounts **only** what the manifest declares. A handler with
  no manifest entry is unreachable.
- They reserve their own permission keys through `adds_permissions` with
  `origin = <slug>`, so the role editor can grant them without the host knowing anything
  about the plugin.
- Their sidebar entries group under one section id rather than scattering under
  "Plugins".
- Tables they create are confined to their own prefix. Nothing else may be created by a
  migration.

## The compute family

**Compute Foundation** is a dependency plugin: it is installed automatically with
**Virtual Machines** or **System Containers**, and removed when the last of them goes. It
holds the shared pools, networks, image catalog and the `agent.rpc` scope, and neither
guest plugin works without it.

## Reporting is also a host

**Reporting** is the only plugin in the repository that hosts an adapter family of its
own: other plugins contribute report sections to it. The Kubernetes, Swarm, KVM and LXC
add-ons all do, which is why a report grows sections as you install add-ons.

## Related

- [Plugin catalog](./catalog.md): every shipped plugin in one table
- [Plugins](./README.md): the section index
- [Documentation index](../README.md): where the six add-on sections are listed
