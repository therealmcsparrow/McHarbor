# Add-on Development

Developer pages for the six add-ons. Each page covers what the add-on registers, its source
layout with line counts, its manifest table, and how to build, sign and fork it. The
operator-facing section for each add-on is linked from the top of its page.

An add-on is a plugin that owns a whole product area, with its own routes, pages, tables,
widgets and nodes, rather than filling one adapter slot. Its developer page therefore describes
a feature, not an adapter. The rules in [Plugin development](../plugins/README.md) apply in
full, and `addon-*` is absolute: nothing outside an add-on imports it.

## Pages

| Add-on | Developer page | Operator section |
|---|---|---|
| Compute Foundation | [compute-foundation.md](./compute-foundation.md) | [Compute Foundation](../../compute-foundation/README.md) |
| Docker Swarm | [docker-swarm.md](./docker-swarm.md) | [Docker Swarm](../../docker-swarm/README.md) |
| Kubernetes | [kubernetes.md](./kubernetes.md) | [Kubernetes](../../kubernetes/README.md) |
| Reporting | [reporting.md](./reporting.md) | [Reporting](../../reporting/README.md) |
| System Containers | [system-containers.md](./system-containers.md) | [System Containers](../../system-containers/README.md) |
| Virtual Machines | [virtual-machines.md](./virtual-machines.md) | [Virtual Machines](../../virtual-machines/README.md) |

## Before changing one

- Virtual Machines and System Containers run only over a **native** remote agent. The
  containerised agent has neither QEMU nor systemd. See
  [Compute Foundation](../../compute-foundation/README.md).
- Rebuilding the McHarbor container restarts it. Never rebuild while a VM import is uploading
  or converting, because the restart kills it.
- An edit to any host package a plugin links, anything under `src/backend/core/plugin/**` in
  particular, changes that package's build ID. Every installed `.so` then refuses to load until
  it is rebuilt and reinstalled. See [Architecture](../plugins/architecture.md).
