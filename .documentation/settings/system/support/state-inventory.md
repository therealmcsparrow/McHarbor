# State inventory

**Enterprise → State inventory** · `/enterprise/state-inventory` · `support.view`

Everything McHarbor keeps in its data directory, measured on request. The page is read-only: it
never deletes, moves or changes anything.

The header shows the data directory's path, the bytes McHarbor uses, the free space on its
volume and the database driver.

## Columns

| Column | Meaning |
|---|---|
| **Size** | Total bytes and file count. A directory with more than 200,000 files is measured partly and shows `+` |
| **Writable** | A probe file could be created and removed (directories), or the file opens for writing (files) |
| **Instance backup** | Whether an [instance backup](../instance-backup/README.md) carries it: **Included**, **Partly**, **Not included**, or **As tables** for the database |
| **Cleanup** | **Keep**, **Safe to remove**, **Pruned by retention** (McHarbor deletes old entries itself), **Recreated on demand**, or **Unknown** for something McHarbor did not create |

## Categories

| Category | Path | Instance backup | Cleanup |
|---|---|---|---|
| Database | `mcharbor.db` (+ `-wal`, `-shm`), or the external driver | As tables | Keep |
| Data encryption key | `.encryption_key` | Included | Keep |
| Server secret | `.secret` | Included | Keep |
| TLS certificates | `tls/` | Partly: the CA always, the serving certificate only when uploaded | Keep |
| Installed plugins | `plugins/` | Not included; reinstalled from the catalog on restore | Keep |
| Plugin catalog cache | `plugin-catalog/` | Not included | Recreated on demand |
| Stacks | `stacks/` | Included | Keep |
| Uploaded files | `files/` | Included | Keep |
| Custom workflow nodes | `custom-nodes/` | Included | Keep |
| Dashboard widgets | `widgets/` | Included | Keep |
| Widget state | `widgets-state.json` | Included | Keep |
| Workflow node state | `workflow-nodes/` | Included | Keep |
| Git working copies | `git/` | Not included | Recreated on demand |
| Container backups | `backups/containers/` | Not included | Pruned by retention |
| Backup staging | `backups/tmp/` | Not included | Safe to remove while no backup runs |
| Instance backups | `instance-backups/` | Not included | Pruned by retention |
| Restore staging | `.restore/` | Not included | Pruned by retention |
| Restore ledger | `restore-ledger.jsonl` | Included | Keep |
| Restore status | `restore-status.json` | Not included | Safe to remove |
| Support bundles | `support-bundles/` | Not included | Safe to remove |
| Other | anything else at the top level | Not included | Unknown |

The *Instance backup* column for the file roots is read from the instance backup's own list, so
the two cannot disagree.

## Route

| Method | Route | Permission |
|---|---|---|
| `GET` | `/api/support/inventory` | `support.view` |
