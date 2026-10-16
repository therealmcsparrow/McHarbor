# Volumes

`/volumes` · `/volumes/:name`

Docker volumes on the selected environment, what mounts them, and what is safe to remove.

## Permissions

| Key | What it unlocks |
|---|---|
| `volumes.view` | See volumes and their detail |
| `volumes.manage` | Create and prune |
| `volumes.delete` | Remove a volume |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Table or card view with an **in use** or **unused** badge per row.
- A detail page with three tabs: Overview, Containers and Configuration.
- Which containers mount the volume, at which path, read-write or read-only.

## Size is often "Not reported"

That is not a fault: the Docker daemon reports a volume's size only when it has collected
usage data for it. The page says so rather than showing a misleading zero.

## Two guards

- A volume a container still mounts cannot be removed: "a container still mounts this
  volume, remove or stop it first".
- A volume marked **protected** cannot be removed at all.

Prune has neither guard in the same sense: it removes everything unreferenced, and an
anonymous volume holding a database that no container currently mounts is unreferenced.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): the mounts tab, and removing volumes with a container
- [Backups](../../backup/screen/README.md): what actually protects the data in a volume
