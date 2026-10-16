# Instance backups

17 endpoints: exporting the whole McHarbor instance to one encrypted file, scheduling and
verifying exports, and restoring from one.

| Key | Covers |
|---|---|
| `instance_backup.view` | List and read exports, schedules, restore status and the restore ledger |
| `instance_backup.manage` | Export, generate a key file, download, verify, delete; schedules |
| `instance_backup.restore` | Upload a file, plan and run a restore |
| `instance_backup.restore_audit` | Restoring audit history, through an approved elevation only |

## Things worth knowing first

**An export is every secret in the instance at once.** Downloading one takes
`instance_backup.manage` and a step-up window on the session (`POST /api/auth/sudo` with
`scope: "instance_backup.download"`), like revealing a vault secret.

**The key file is shown once.** `POST /api/instance-backups/key-file` generates it, returns it,
and never stores it. Without it, or the passphrase, an export cannot be read.

**Plan before you restore.** `POST /api/instance-backups/restore/plan` reads and checks the
whole file and writes nothing; `POST /api/instance-backups/restore` takes the same body. A
file from elsewhere is first sent raw to `PUT /api/instance-backups/uploads`, and its
`uploadId` goes in `source`.

**Audit history needs a second person.** `includeAudit: true` is refused unless the caller holds
an active, approved elevation for `instance_backup.restore_audit`; a standing grant is not
enough.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/instance-backups` | `instance_backup.view` | Lists instance exports | — |
| `POST` | `/api/instance-backups` | `instance_backup.manage` | Starts an export of the whole instance, encrypted with a passphrase or a key file, and runs it in the background; `409` `busy` while another runs | — |
| `POST` | `/api/instance-backups/key-file` | `instance_backup.manage` | Generates a key file; it is returned once and never stored | — |
| `POST` | `/api/instance-backups/restore` | `instance_backup.restore` | Restores from an export or an upload. `includeAudit` also needs an approved elevation for `instance_backup.restore_audit` | — |
| `GET` | `/api/instance-backups/restore/ledger` | `instance_backup.view` | The restore history, kept apart so it survives the audit log being replaced | — |
| `POST` | `/api/instance-backups/restore/plan` | `instance_backup.restore` | Reads and checks the whole file and returns what a restore would do; writes nothing | — |
| `GET` | `/api/instance-backups/restore/status` | `instance_backup.view` | Reports the running or most recent restore | — |
| `GET` | `/api/instance-backups/schedules` | `instance_backup.view` | Lists export schedules | — |
| `POST` | `/api/instance-backups/schedules` | `instance_backup.manage` | Creates an export schedule | — |
| `DELETE` | `/api/instance-backups/schedules/{id}` | `instance_backup.manage` | Removes an export schedule | `{id}` |
| `PUT` | `/api/instance-backups/schedules/{id}` | `instance_backup.manage` | Replaces an export schedule | `{id}` |
| `POST` | `/api/instance-backups/schedules/{id}/run` | `instance_backup.manage` | Runs a schedule now | `{id}` |
| `PUT` | `/api/instance-backups/uploads` | `instance_backup.restore` | Stores an uploaded export for a restore and returns its `uploadId` and unauthenticated manifest. The body is the raw file | — |
| `DELETE` | `/api/instance-backups/{id}` | `instance_backup.manage` | Deletes an export | `{id}` |
| `GET` | `/api/instance-backups/{id}` | `instance_backup.view` | Returns one export | `{id}` |
| `GET` | `/api/instance-backups/{id}/download` | `instance_backup.manage` | Streams the export file. Needs a step-up window on the session as well as the key | `{id}` |
| `POST` | `/api/instance-backups/{id}/verify` | `instance_backup.manage` | Rehearses a completed export in the background (restored into a throwaway database and checked) and stores the outcome on the row | `{id}` |

## Request bodies

**`POST` `/api/instance-backups`**: `passphrase`?, `keyFile`?, `includeMetrics`, `note`?, `locationIds`?

`locationIds` is a list of storage-location ids the finished export is copied to. Every one is
attempted even after another fails, and each outcome is returned on the export as
`destinations[]`: `locationName`, `locationType`, `status` (`uploading` / `success` /
`failure`), `remoteKey`, `error`, `uploadedAt`, `bytesUploaded`, `bytesTotal`. The older
single-destination fields `locationId`, `locationKey`, `locationStatus` and `locationError`
remain on the export as a summary of the first success, or of the first failure when there is
none.

**`POST` `/api/instance-backups/restore`**: `source`, `passphrase`?, `keyFile`?, `mode`, `categories`?, `includeAudit`, `conflict`?. `source` is `exportId`? or `uploadId`?; the modes, categories and conflict policies are in [Restore](../../settings/system/instance-backup/restore.md).

**`POST` `/api/instance-backups/restore/plan`**: `source`, `passphrase`?, `keyFile`?, `mode`, `categories`?, `includeAudit`, `conflict`?

**`POST` `/api/instance-backups/schedules`**: `name`, `cron`, `enabled`, `passphrase`?, `keyFile`?, `includeMetrics`, `locationIds`?, `retentionCount`, `retentionDays`, `verifyAfter`

**`PUT` `/api/instance-backups/schedules/{id}`**: `name`, `cron`, `enabled`, `passphrase`?, `keyFile`?, `includeMetrics`, `locationIds`?, `retentionCount`, `retentionDays`, `verifyAfter`

`locationIds` replaces the schedule's whole destination set on every save, so a location left
out is removed. An id that no longer names a location is dropped rather than refusing the save.

**`POST` `/api/instance-backups/{id}/verify`**: `passphrase`?, `keyFile`?

## Related

- [Export](../../settings/system/instance-backup/export.md)
- [Restore](../../settings/system/instance-backup/restore.md): modes, categories, conflicts
- [Schedules and verification](../../settings/system/instance-backup/schedules-and-verification.md)
- [Backups, restore and storage](./backups.md): container backups, and the storage locations schedules write to
