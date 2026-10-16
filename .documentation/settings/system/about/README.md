# About

**Settings → System → About** · `?tab=about`

Version, dependencies, update checking, and factory reset.

## Permissions

| Key | What it unlocks |
|---|---|
| _readable in Settings_ | Version and dependencies |
| `settings.manage` | Update, and factory reset |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- What is running: version, platform, and the backend and frontend dependency lists.
- Checking for and installing a newer McHarbor.
- Factory reset.

If the version and dependency data cannot be loaded, the tab shows the server's own error message
with a **Retry** button above the rest of the page, rather than an empty panel.

## Why the dependency lists are here

They answer "does this install contain the library that was in the news this morning"
without shelling into the container.

## Factory reset

> Erase all McHarbor data and return the installation to its first-run state. Containers,
> images, and volumes on your hosts are not touched.

The confirmation is explicit: "Every environment, user, setting, schedule, and history entry
stored by McHarbor is deleted permanently. **This cannot be undone.**" You type a phrase to
confirm, and McHarbor restarts into the setup flow.

Two things worth knowing before pressing it: **your containers survive** (this deletes
McHarbor’s knowledge of them, not them), and **encrypted backups do not become readable
again by resetting**, because the backup key is a Docker secret rather than McHarbor state.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Licence](../license/README.md): entitlement and seats
- [Updates → McHarbor](../../updates/mcharbor/README.md): the same update controls in the Updates group
- [Getting started → Upgrading](../../../getting-started/upgrading/README.md): the whole upgrade path
