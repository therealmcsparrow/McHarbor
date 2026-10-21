# scripts/development

Host-side helpers for a local McHarbor dev container. They talk to the
running container's SQLite database (`docker compose exec … sqlite3`),
so `docker compose up -d` must be running first.

## Local mounts: storage location + catalogs

| Script | Effect |
|---|---|
| `add-dev-storage-and-catalogs.sh` | Registers `/mnt/backup` as a `local` storage location and `/mnt/app-catalog` (docker_app) + `/mnt/plugin-catalog` (plugin) as catalogs. |
| `remove-dev-storage-and-catalogs.sh` | Removes the same three rows so a promoted database is production-clean. |

```bash
bash scripts/development/add-dev-storage-and-catalogs.sh
bash scripts/development/add-dev-storage-and-catalogs.sh --dry-run

bash scripts/development/remove-dev-storage-and-catalogs.sh
bash scripts/development/remove-dev-storage-and-catalogs.sh --yes
```

Both are idempotent. Overrides: `MCHARBOR_SERVICE`, `MCHARBOR_DB`,
`MCHARBOR_DEV_BACKUP_PATH`, `MCHARBOR_DEV_APP_CATALOG_PATH`,
`MCHARBOR_DEV_PLUGIN_CATALOG_PATH`.

### Why SQL rather than the REST API

Storage locations of type `local` are protected by the service layer
(`ErrLocalStorageProtected` in
`src/backend/modules/storage_locations/service.go`) — the API refuses to
delete or disable them, so the remove side has no REST path. Both
scripts therefore write SQL, and the remove side enables
`PRAGMA foreign_keys` so the backup-plan cascades actually fire.

### Re-seed caveat

Removing the rows cleans the current database only:

- migration `051_protect_local_storage.sql` recreates the `/mnt/backup`
  row on a fresh data volume;
- `catalogs.SeedLocalCatalogs` re-registers `/mnt/app-catalog` and
  `/mnt/plugin-catalog` on **every** boot when those paths exist inside
  the container and contain at least one `.json` file.

A genuinely clean production image also drops the corresponding bind
mounts from `docker-compose.yml`.
