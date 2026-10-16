# Pre-upgrade check

**Settings → Updates → Check for updates → Pre-upgrade check** · `updates.view`

When a newer McHarbor release is available, the update card offers **Pre-upgrade check** next to
**View release**. It measures this instance against the new version and says what an admin
should know before pressing **Update now**. It changes nothing and never blocks the update.

## What it checks

| Check | What it reports |
|---|---|
| **Migrations for the target version** | The target image's migrations cannot be known until it is pulled, and the check says so. It still warns when this build has migrations pending, or the database has migrations this build does not know |
| **Data directory** | Writable, and at least 2 GB free for the image pull and a backup |
| **Docker reachability** | The Docker hosts McHarbor reaches directly answer; the self-update talks to the local one |
| **Data encryption key** | A key that only exists in `DATA_DIR/.encryption_key` is lost with the volume |
| **Backup encryption key** | Whether backups can be encrypted before the upgrade |
| **Plugin compatibility** | Each enabled plugin's `mcharbor:` range from its installed `plugin.yaml`, measured against the target: **Compatible**, **Not compatible** or **Unknown** (no readable range) |
| **Running backups and exports** | Container backup runs, instance exports or verifications and support bundles still running; the restart would interrupt them |

Plugin binaries are also bound to the exact McHarbor build. Plugins shipped with the image are
refreshed by it; any other plugin must be updated from the Plugin Store after the upgrade.

## Rollback guidance

The dialog ends with how to go back, depending on the database:

- **SQLite**: take an instance backup first. Migrations only move forward; to roll back, stop
  McHarbor, restore the backup or a copy of the data volume, and start the previous image tag.
- **PostgreSQL / MySQL**: take a database dump first. To roll back, stop every node, restore the
  dump and start the previous image tag on all nodes together.
- The previous image stays in the local image cache.
- Keep `ENCRYPTION_KEY`, `MCHARBOR_SECRET` and the backup key unchanged across the upgrade and
  any rollback.

## Route

| Method | Route | Permission |
|---|---|---|
| `GET` | `/api/support/pre-upgrade?target=2026.10.01` | `updates.view` |

An empty `target` checks the running version. A target that is not a version string answers
`400` with `data.reason = "invalid_version"`.
