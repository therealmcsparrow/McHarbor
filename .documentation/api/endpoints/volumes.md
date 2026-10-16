# Volumes

5 endpoints. Volumes are addressed by **name**, not by id, because that is what Docker uses.

| Key | Covers |
|---|---|
| `volumes.view` | List and inspect |
| `volumes.manage` | Create, prune |
| `volumes.delete` | Remove |

## Things worth knowing first

**`POST /api/volumes/prune` removes every unused volume in the environment**, not a selected
set. An anonymous volume that a stopped container will be restarted onto counts as unused.

**Removing a volume destroys its data.** There is no soft delete and no undo;
`?force=true` only bypasses Docker's in-use check.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/volumes` | `volumes.view` | Returns all volumes | — |
| `POST` | `/api/volumes` | `volumes.manage` | Creates a new volume | — |
| `POST` | `/api/volumes/prune` | `volumes.manage` | Removes unused volumes | — |
| `DELETE` | `/api/volumes/{name}` | `volumes.delete` | Removes a volume | `{name}`; `?force=` |
| `GET` | `/api/volumes/{name}` | `volumes.view` | Returns detailed volume information | `{name}` |


## Request bodies

**`POST` `/api/volumes`**: `name`, `driver`?, `driverOpts`?, `labels`?

## Related

- [Containers](./containers.md): mounting a volume
- [Backups](./backups.md): backing a volume up
- [Volumes screen](../../screens/volumes/README.md)
