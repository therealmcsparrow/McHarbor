# Docker

`/docker`

What the Docker daemon on the selected environment actually is, the registry credentials
McHarbor pulls with, and how much room is left.

## Permissions

| Key | What it unlocks |
|---|---|
| _session only_ | The Docker Info section |
| `registry.view` | See configured registries |
| `registry.manage` | Add, edit, test and remove a registry |
| `host.view` | The Resources section |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Three sections: **Docker Info**, **Registries** and **Resources**.
- Everything `docker info` reports, grouped into server, resources, hardware, storage, runtime, plugins, object counts, security, swarm and labels.
- Docker's own disk usage, and how much of it is reclaimable.

## One caveat the page states itself

**Hardware is the host running McHarbor, not the selected environment.** On a platform
where the device inventory cannot be read, it says that too rather than showing nothing.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Images](../images/README.md): pulling with these credentials
- [Environments](../environments/README.md): which daemon this page describes
