# Backups, restore and storage

46 endpoints: backup plans and their runs, the restore browser, the log feed, and the storage
destinations everything is written to.

| Key | Covers |
|---|---|
| `backups.view` | Plans, runs, snapshot configs, options |
| `backups.manage` | Create and edit plans, run backups, cancel, retry an upload, snapshot configs |
| `backups.delete` | Delete plans, runs and snapshot configs |
| `backups.download` | Download an archive |
| `restore.view` | Browse inside an archive |
| `restore.run` | Restore into a container |
| `backup_log.view` | Read the log feed |
| `settings.view` or `storage_locations.view` | Read storage destinations |
| `settings.manage` or `storage_locations.manage` | Create, edit, test, migrate, OAuth |
| `settings.manage` or `storage_locations.delete` | Delete a destination |

## Things worth knowing first

**Archives are encrypted.** A backup needs the backup encryption key to exist
(`err.container.backup_key_missing`), and restoring or browsing one needs the key supplied as
`secretKey` unless the running key already matches. See [Settings](../settings/README.md)
for generating and installing it.

**Browsing an archive is four endpoints, not one.** `restore-options` lists what the archive
holds; `restore-filesystem-entries`, `restore-mount-entries` and `restore-config-entries`
each return the first-level children of a path inside one part of it, so the restore dialog
expands a tree lazily instead of unpacking a multi-gigabyte tar to draw a list. All four are
`POST` because they carry the key in the body rather than in the URL.

**A run is polled, not streamed.** `GET /api/container-backups/runs/{runId}` is a plain read
with no audit entry and the same view permission as the list; the restore dialog calls it
repeatedly for progress.

**A plan can have several destinations.** `storageLocationIds` is the current field;
`storageLocationId` is the single-destination form it grew out of. A destination whose upload
failed can be retried on its own with
`POST /api/container-backups/runs/{runId}/destinations/{destinationId}/retry-upload`, which
returns immediately with that destination in `uploading` and does the work in the background.

**Snapshots are a consistency mechanism, not a destination.** A snapshot config names a
driver (ZFS, LXD, or one a plugin contributes) that can freeze the filesystem while the
archive is taken. `GET /api/container-backups/snapshot-drivers` asks the environment what it
can actually do; on an agent environment the agent probes its own host, so the operator is
told what exists rather than having to guess before saving.

**Deleting a snapshot config does not break plans that point at it.** The foreign key nulls
the reference and those plans fall back to pausing the container instead.

**Snapshot config secrets are encrypted and never returned.** A field the driver's `ConfigSchema`
marks secret (a NAS API key or password) is encrypted before it is stored. `GET`, `POST` and `PUT`
on `/api/container-backups/snapshot-configs` leave those values out of `config` and list the stored
ones by key in `secretsSet`. A `PUT` that omits a secret, or sends it blank, keeps the stored value.
Saving for a driver that is not loaded (its plugin is not installed or not enabled) is refused with
`400 err.plugin.not_installed`, because without the driver McHarbor cannot tell which fields are
secret. A row saved before this change is encrypted on the first start that has an encryption key;
a row whose driver is not loaded at that start waits for a start where it is.

**Storage credentials are write-only on the ordinary read path.**
`GET /api/storage-locations/{id}` omits them by design.
`GET /api/storage-locations/{id}/secrets` is a separate, manage-gated endpoint that exists so
an edit dialog can repopulate its fields without the operator re-entering every credential
for a destination that already works.

**Cloud destinations authorise over OAuth.** `POST /api/storage-locations/{id}/oauth/authorize`
builds the consent URL (the owning plugin supplies the redirect target, scopes and prompt
options), and `GET /api/storage-locations/oauth/callback` stores the tokens afterwards.

**Capacity reporting is optional and says so.** `GET /api/storage-locations/{id}/usage`
answers from the filesystem for a local destination and from the owning plugin's quota call
for every other type. A plugin that reports no capacity (because the protocol has none, as
with FTP) returns `supported: false` with zeroed byte counts and **200**, not an error, so
the settings card renders without a bar instead of failing.

Plans and runs store the container's id. When Docker recreates a container (`docker compose
up`, a configuration restore), the new container gets a new id under the same name. McHarbor
watches container `create`, `rename` and `start` events on every environment with **Track container events**
on, and moves every plan and run in that environment that names the container to
the new id as the event arrives. Reading a plan or run also re-resolves a stale id.

**`POST /api/container-backups/admin/relink-all`** is the manual fallback, for environments that
do not track container events: every plan and run gets its container id re-resolved against the
live orchestrator and persisted if stale.
## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `DELETE` | `/api/backup-logs` | `settings.manage` | Deletes every container_backup_log row older than the configured retention cutoff | `?vacuum=` |
| `GET` | `/api/backup-logs` | `backup_log.view` | Returns the paginated backup log feed | — |
| `POST` | `/api/backup-logs` | `backups.manage` | Lets admin tooling (e.g | — |
| `GET` | `/api/container-backups` | `backups.view` | Lists backup plans for an environment and optional container | `?containerId=` |
| `POST` | `/api/container-backups` | `backups.manage` | Creates a backup plan | — |
| `POST` | `/api/container-backups/admin/relink-all` | `backups.manage` | Runs a one-shot reconciliation pass: every plan and run gets its container_id resolved against the live orchestrator and persisted if stale | — |
| `GET` | `/api/container-backups/internal/agent-archives/{transferId}` | — | Streams a complete encrypted archive to an agent | `{transferId}` |
| `PUT` | `/api/container-backups/internal/agent-archives/{transferId}` | — | Receives a complete encrypted archive from an agent | `{transferId}` |
| `GET` | `/api/container-backups/internal/transfers/{transferId}` | — | Streams one prepared restore archive entry to a connected agent | `{transferId}` |
| `GET` | `/api/container-backups/provider-plans` | `backups.view` | Returns the backup schedules every other subsystem keeps, with their sources | — |
| `GET` | `/api/container-backups/provider-runs` | `backups.view` | Returns the backups every other subsystem took, with their sources | — |
| `GET` | `/api/container-backups/runs` | `backups.view` | Lists recent backup runs for an environment | `?containerId=` |
| `DELETE` | `/api/container-backups/runs/{runId}` | `backups.delete` | Deletes a completed or failed backup run and its local archive | `{runId}` |
| `GET` | `/api/container-backups/runs/{runId}` | `backups.view` | Returns one backup or restore run | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/cancel` | `backups.manage` | Cancels a running backup or restore | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/destinations/{destinationId}/retry-upload` | `backups.manage` | Re-uploads the on-disk archive of an existing backup run to one previously-failed storage destination | `{destinationId}`, `{runId}` |
| `GET` | `/api/container-backups/runs/{runId}/download` | `backups.download` | Streams a completed backup archive | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/restore` | `restore.run` | Restores a completed backup archive to its original container. Configuration recreates the container; with the filesystem layer in the same selection it is restored afterwards into the recreated container | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/restore-config-entries` | `restore.view` | Returns the synthetic config tree children under `parentKey` for the named run | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/restore-filesystem-entries` | `restore.view` | Returns the first-level children of `innerPath` inside `container/filesystem.tar` for the named run | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/restore-mount-entries` | `restore.view` | Returns the first-level children of `innerPath` inside `mounts/<safeTarget>.tar` for the named run | `{runId}` |
| `POST` | `/api/container-backups/runs/{runId}/restore-options` | `restore.view` | Returns restorable entries for a completed backup archive | `{runId}` |
| `GET` | `/api/container-backups/snapshot-configs` | `backups.view` | Returns the snapshot configurations for the selected environment | — |
| `POST` | `/api/container-backups/snapshot-configs` | `backups.manage` | Creates a snapshot configuration | — |
| `DELETE` | `/api/container-backups/snapshot-configs/{id}` | `backups.delete` | Removes a snapshot configuration | `{id}` |
| `PUT` | `/api/container-backups/snapshot-configs/{id}` | `backups.manage` | Updates a snapshot configuration | `{id}` |
| `POST` | `/api/container-backups/snapshot-configs/{id}/probe` | `backups.manage` | Runs the driver's capability check and stores the result, which is what the Test button reports | `{id}` |
| `GET` | `/api/container-backups/snapshot-drivers` | `backups.view` | Reports which snapshot drivers this environment can use | — |
| `GET` | `/api/container-backups/snapshots/recent` | `backups.view` | Lists the snapshots recent backup runs took | `?limit=` |
| `DELETE` | `/api/container-backups/{planId}` | `backups.delete` | Deletes a backup plan | `{planId}` |
| `PUT` | `/api/container-backups/{planId}` | `backups.manage` | Updates a backup plan | `{planId}` |
| `POST` | `/api/container-backups/{planId}/run` | `backups.manage` | Runs a saved backup plan | `{planId}` |
| `GET` | `/api/containers/{id}/backups/options` | `backups.view` | Returns backup choices for a container | `{id}` |
| `POST` | `/api/containers/{id}/backups/restore-upload` | `restore.run` | Stores an uploaded archive and restores all of it in one call, for scripts with no selection step | `{id}`; multipart: `file`, `secretKey` |
| `POST` | `/api/containers/{id}/backups/upload` | `restore.run` | Stores an uploaded archive as a backup run of the container after reading it with the key, without restoring; the restore-options and restore routes then work on that run. A wrong key or unreadable archive is refused and nothing is kept. Audited as `backup.upload` | `{id}`; multipart: `file`, `secretKey`; returns the run |
| `POST` | `/api/containers/{id}/backups/run` | `backups.manage` | Runs a manual backup for one container | `{id}` |
| `GET` | `/api/storage-locations` | `settings.view` or `storage_locations.view` | Returns all storage locations | — |
| `POST` | `/api/storage-locations` | `settings.manage` or `storage_locations.manage` | Creates a storage location | — |
| `GET` | `/api/storage-locations/oauth/callback` | `settings.manage` or `storage_locations.manage` | Stores delegated provider tokens after consent | `?code=`, `?state=` |
| `DELETE` | `/api/storage-locations/{id}` | `settings.manage` or `storage_locations.delete` | Removes a storage location | `{id}` |
| `GET` | `/api/storage-locations/{id}` | `settings.view` or `storage_locations.view` | Returns a storage location | `{id}` |
| `PUT` | `/api/storage-locations/{id}` | `settings.manage` or `storage_locations.manage` | Updates a storage location | `{id}` |
| `POST` | `/api/storage-locations/{id}/migrate-container-backups` | `settings.manage` or `storage_locations.manage` | Copies existing completed backups to a local storage location | `{id}` |
| `POST` | `/api/storage-locations/{id}/oauth/authorize` | `settings.manage` or `storage_locations.manage` | Creates a delegated provider consent URL | `{id}` |
| `GET` | `/api/storage-locations/{id}/secrets` | `settings.manage` or `storage_locations.manage` | Returns the decrypted credential columns for a single storage location | `{id}` |
| `POST` | `/api/storage-locations/{id}/test` | `settings.manage` or `storage_locations.manage` | HandleOAuthCallback stores delegated provider tokens after consent | `{id}` |
| `GET` | `/api/storage-locations/{id}/usage` | `settings.view` or `storage_locations.view` | Returns used / total capacity for one storage location so the settings card can render a usage bar | `{id}` |


## Request bodies

**`POST` `/api/container-backups`**: `name`, `containerId`, `storageLocationId`, `storageLocationIds`, `includeConfig`, `includeLogs`, `includeFilesystem`, `includeImage`, `selectedMounts`, `selections`?, `logTailLines`, `cron`, `enabled`, `retentionCount`, `retentionDays`, `consistencyMode`, `snapshotConfigId`

**`POST` `/api/container-backups/runs/{runId}/restore`**: `secretKey`?, `restoreItems`?

**`POST` `/api/container-backups/runs/{runId}/restore-config-entries`**: `secretKey`?, `parentKey`?

**`POST` `/api/container-backups/runs/{runId}/restore-filesystem-entries`**: `secretKey`?, `innerPath`?

**`POST` `/api/container-backups/runs/{runId}/restore-mount-entries`**: `secretKey`?, `mountTarget`, `innerPath`?

**`POST` `/api/container-backups/runs/{runId}/restore-options`**: `secretKey`?

**`POST` `/api/container-backups/snapshot-configs`**: `environmentId`, `name`, `driver`, `pluginSlug`, `enabled`?, `poolName`, `datasetPrefix`, `sshTarget`, `useLxd`?, `config`

**`PUT` `/api/container-backups/snapshot-configs/{id}`**: `name`?, `enabled`?, `poolName`?, `datasetPrefix`?, `sshTarget`?, `useLxd`?, `config`?

**`PUT` `/api/container-backups/{planId}`**: `name`?, `storageLocationId`?, `storageLocationIds`?, `includeConfig`?, `includeLogs`?, `includeFilesystem`?, `includeImage`?, `selectedMounts`?, `selections`?, `logTailLines`?, `cron`?, `enabled`?, `retentionCount`?, `retentionDays`?, `consistencyMode`?, `snapshotConfigId`?

**`POST` `/api/containers/{id}/backups/run`**: `name`, `storageLocationId`, `storageLocationIds`, `includeConfig`, `includeLogs`, `includeFilesystem`, `includeImage`, `selectedMounts`, `selections`?

**`POST` `/api/storage-locations`**: `name`, `locationType`, `enabled`, `host`, `port`, `basePath`, `region`, `bucket`, `endpoint`, `tenantId`, `siteUrl`, `driveId`, `shareName`, `domain`, `username`, `authMethod`, `tlsMode`, `passiveMode`, `password`, `privateKey`, `passphrase`, `caCertificate`, `clientCertificate`, `clientKey`, `accessKeyId`, `secretAccessKey`, `clientId`, `clientSecret`, `refreshToken`, `token`, `config`?

**`PUT` `/api/storage-locations/{id}`**: `name`?, `locationType`?, `enabled`?, `host`?, `port`?, `basePath`?, `region`?, `bucket`?, `endpoint`?, `tenantId`?, `siteUrl`?, `driveId`?, `shareName`?, `domain`?, `username`?, `authMethod`?, `tlsMode`?, `passiveMode`?, `password`?, `privateKey`?, `passphrase`?, `caCertificate`?, `clientCertificate`?, `clientKey`?, `accessKeyId`?, `secretAccessKey`?, `clientId`?, `clientSecret`?, `refreshToken`?, `token`?, `config`?

## Related

- [Containers](./containers.md): what is being backed up
- [Settings](../settings/README.md): the backup encryption key
- [Backups screen](../../backup/screen/README.md)
