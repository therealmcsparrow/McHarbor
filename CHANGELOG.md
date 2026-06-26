# Changelog

All notable changes to McHarbor are documented in this file.

## [1.6.3] - 2026-06-26

### Added
- Added **Clear now** buttons next to the Audit Log Retention and Activity Log Retention fields in **Settings → General → Data Retention**. Each button runs the retention purge immediately and, when invoked, schedules a background `PRAGMA wal_checkpoint(TRUNCATE)` plus `VACUUM` so the on-disk database file actually shrinks (SQLite only marks pages as free in the response to `DELETE`; without `VACUUM` the `.db` file stays the same size). New endpoints `DELETE /api/audit` and `DELETE /api/activity` run the same retention-purge as the existing automatic loops; both accept an optional `?vacuum=true` query parameter. Endpoints `GET /api/audit/prunable` and `GET /api/activity/prunable` were evaluated and removed because the count fetch was as expensive as the prune itself. The toast reports the actual deleted count from the DELETE response and notes when a background vacuum is in flight. New i18n keys `toast.auditLogPurged`, `toast.auditLogPurgedVacuuming`, `toast.activityLogPurged`, `toast.activityLogPurgedVacuuming`, `retention.clearNow`, `retention.clearNowHint`, `retention.clearAuditTitle`, `retention.clearAuditDescription`, `retention.clearActivityTitle`, `retention.clearActivityDescription`, `retention.keepForever` shipped in all six locales (en, nl, de, es, fr, pt).
- Added a **date-range picker** to the Activity and Audit Log pages. Both pages now expose a dropdown with 15 presets: All time, Last 15 minutes, Last hour, Last 24 hours, Today, Yesterday, This week, Last week, Last 7 days, This month, Last month, Last 30 days, This year, Last year, and Custom range. The Custom range option opens two HTML5 date inputs. The selected window is forwarded to the backend as `from` / `to` RFC3339 query parameters and applied at the SQL layer via an indexed `WHERE timestamp < ?` comparison on the existing `timestamp` index.
- Added **lazy loading** (intersection-observed infinite scroll) to the Activity and Audit Log tables. The previous behavior capped each list at 100 rows; the new `useInfiniteQuery` paginates 100 rows per request and fetches the next page when the user scrolls within 200 px of the table bottom. The "End of list" sentinel appears when no further pages are available. `ParsePagination`'s server-side cap was raised from 100 to 1000 to keep roundtrips reasonable for the larger windows.
- Added **container pause/unpause** to the local container backup flow. The container is paused before `ContainerExport`, `CopyFromContainer`, `ContainerLogs`, and `ImageSave` run, and unpaused via a defer that uses a fresh background context so the container is always released even if the main operation context was cancelled. The pause is best-effort: Windows containers and runtimes that reject pause log a warning and proceed without snapshot consistency. This eliminates the silent data-corruption class where mid-write files ended up in the archive. The same pause/unpause is mirrored in the agent-side `runBackupArchiveAndUploads` flow and exposed on the agent via new `Proxy.PauseContainer` / `Proxy.UnpauseContainer` methods that hit `POST /containers/{id}/pause` and `/unpause` on the local Docker socket.
- Added **parallel destination uploads**. `Service.uploadArchiveDestinations` now spawns one goroutine per destination via `sync.WaitGroup` instead of iterating sequentially. A plan with `[local, OneDrive, SharePoint]` now takes `max(...)` wall time instead of the sum. Per-destination status rows in `container_backup_run_destinations` are written independently.
- Added **retry-with-backoff on destination uploads**. Transient failures (connection refused / reset / i/o timeout / TLS handshake / EOF / broken pipe / Microsoft Graph 5xx and 429) are retried up to 3 times with exponential backoff 2 s → 8 s → 32 s. Permanent failures (auth, validation, 4xx) fail immediately. The retry loop runs in the per-destination goroutine so each destination retries independently. The same retry policy is mirrored in the agent's `uploadAgentBackupArchive`.
- Added **per-destination upload progress** via two new columns `bytes_uploaded` / `bytes_total` on `container_backup_run_destinations` (migration `055`). The local and OneDrive upload paths now write the running byte counter to the destination row on every progress callback, so the UI can show "Uploading to OneDrive (450 MB / 1.2 GB)" rather than the previous opaque "uploading" stage. The new `BackupRunDestination` fields are returned by `GET /api/container-backups/runs/{runId}`.
- Added **container-name resolution** for stale container ids (Compose rotation). `Service.resolveContainerForBackup` tries `ContainerInspect(id)` first; on `No such container` (404) it falls back to `ContainerList` filtered by name and uses the live id for this run. The plan row itself is not rewritten — the next run retries the lookup. This rescues plans that captured a `container_id` before a `docker compose up` rotated the id.
- Added **`log_tail_lines` per plan** (migration `056`). Default 10000 lines, 0 = legacy "all" behavior. `ContainerLogs` now passes `Tail: strconv.Itoa(plan.LogTailLines)` (or `"all"` if 0) so chatty containers can't produce multi-GB log archives. The plan input types (`CreateBackupPlanInput`, `UpdateBackupPlanInput`) carry the field and the SQL reads/writes go through the standard scan/insert paths.
- Added **TCP keepalive on the agent WebSocket** at `src/backend/modules/agent/handler.go::HandleAgentWS`. After upgrade, the underlying `*net.TCPConn` has `SetKeepAlive(true)` and `SetKeepAlivePeriod(30 * time.Second)` applied. Combined with the new stuck-run reaper threshold drop (see Fixed below) this brings agent disconnect detection from "until the OS TCP keepalive fires (often 2+ hours)" down to ~30 s.
- Added a **dedicated recovery loop** in `main.go` that scans `container_backup_runs` for stale "running" rows every 30 s (previously this piggybacked on the 60 s scheduler tick). The threshold above which a run is considered abandoned dropped from 6 minutes to 2 minutes. Together this shrinks the worst-case stale-UI window from ~7 min to ~2.5 min.

### Changed
- Bumped McHarbor and the remote agent to `1.6.3`.
- Optimized the audit-log and activity-event retention purge SQL. The previous query `DELETE FROM audit_logs WHERE julianday(timestamp) < julianday('now', '-N days')` compared the stored RFC3339 timestamp against an SQLite-native cutoff, which (a) produced wrong results on the boundary date because `' ' < 'T'` lexicographically (`2026-05-25T…` < `2026-05-25 …` is false but should be true) and (b) could not use the `idx_audit_logs_timestamp` / `idx_container_events_timestamp` indexes because `julianday(timestamp)` is a function expression. The new query `DELETE FROM <table> WHERE timestamp < ?` with the cutoff computed in Go as `time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)` compares same-format strings (correct) and uses the timestamp index for a range scan (fast). Both `core/audit/audit.go::Prune` and `modules/activity/collector.go::prune` now use the indexed form.
- The McHarbor SQLite connection pool was tuned for the deadlock class that locked the process during a degraded 6 GB database. `MaxOpenConns` raised from 1 to 8 (SQLite WAL supports many concurrent readers + a single writer), `MaxIdleConns` raised from 1 to 4, and `SetConnMaxIdleTime(5 * time.Minute)` added. `modules/scans/registry.go` adds an `RWMutex` + `Reload(clairURL)` so the scanner registry is mutated atomically instead of under a reader's foot.
- Replaced the explicit `cli.ContainerExport` → temp file → re-read → `tar.Writer` flow for backup entries with a fast-path that buffers in `bytes.Buffer` for any entry ≤ 64 MiB (`ContainerLogs`, most mount copies) and keeps the disk spool for `ContainerExport` / `ImageSave` (which routinely exceed that threshold). The tar-package API requires the entry size in the header before the data is written, so the disk spool cannot be eliminated entirely without a custom writer; the fast-path halves the I/O for small entries and keeps the existing behavior for large ones.
- `Service.writeArchiveEntries` and `src/agent/backup.go::writeAgentBackupStream` both now do their reads once through the new spool helper.
- `Service.executeRun` and `Service.executeAgentLocalBackup` now wrap the entire archive-write block in `pauseContainerForBackup` / `unpauseContainerAfterBackup` / `pauseAgentContainerForBackup` / `unpauseAgentContainerAfterBackup` helpers respectively. Unpause uses a fresh `context.WithTimeout(context.Background(), 30*time.Second)` so a cancelled outer context cannot leak the container in paused state.

### Fixed
- Fixed a startup-time deadlock that crashed the McHarbor process on degraded databases. The previous `database.SetMaxOpenConns(1)` + `MaxIdleConns(1)` forced every concurrent goroutine (scanner registry load, metrics collector, activity collector, alerts engine, autoheal engine, backup scheduler, rows-cleanup) to serialize through one SQLite connection; on a multi-GB DB that single connection was held long enough during `cli.ContainerInspect` that Go's runtime deadlock detector fired (`fatal error: all goroutines are asleep - deadlock!`) and Docker killed the container, restarting every 30 s. The pool tuning above fixes this. The scanner registry load (`scans.Mount`) is now also asynchronous — the HTTP server binds immediately while the registry loads in a background goroutine with retry + backoff — so a slow `ReadScannerSettings` query can no longer delay startup.
- Fixed the audit-log and activity-event retention prune to actually delete rows. The previous query "succeeded" (zero rows affected) because of the format-mismatch bug documented above. After the format fix, a 30-day retention now correctly removes everything older than 30 days; in the user's case the purge deleted 117,789 stale container-event rows in 6.7 s on an indexed query, dropping the `.db` size from 6.16 GB to 1.81 GB once a `VACUUM` was run.
- Fixed the "Clear now" button leaving the database file unchanged. The DELETE statement does not shrink the SQLite file; the freed pages stay in the file until `VACUUM` rewrites it. The button now always triggers a synchronous `PRAGMA wal_checkpoint(TRUNCATE)` and spawns a background `VACUUM` goroutine (gated by a mutex so concurrent triggers don't race). The `core/db/compact.go::CompactManager` coordinates the work: it logs `starting database vacuum` when kicked off and `vacuum completed duration=… size_bytes=…` when done. The `?vacuum=true` query parameter on `DELETE /api/audit` and `DELETE /api/activity` exposes the same plumbing.
- Fixed the "agent path" being skipped entirely when the agent is disconnected. `Service.executeRun` previously returned early on `s.client(envID)` failure for an agent env, leaving the run in a "queued" state with no fallback to the local pipeline. The `tryAgentClient` flow now falls back to the local-server pipeline (`writeArchive` + `uploadArchiveDestinations`) when the agent is not in the pool, so a temporarily disconnected agent no longer wedges the run.
- Fixed backup plans breaking silently after `docker compose up`. Plans stored `container_id` only; Compose recreates the container with a fresh id on every restart, so the next backup would fail at the `inspecting` stage with "No such container". With `resolveContainerForBackup` (see Added), the plan falls back to name lookup and the run succeeds.
- Fixed the stuck-run reaper firing 6 minutes late on average. The previous `backupRunProgressStaleAfter = 6 * time.Minute` only ran on the 60 s scheduler tick, so a run that crashed at 09:00:00 could stay in "running" until 09:06:30. The new threshold + dedicated loop halves the window to ~2.5 min.
- Fixed `runCancelers` accumulation under heavy backup churn. The map was already cleaned up in the deferred goroutine exit; verified and re-confirmed.

## [1.6.2] - 2026-06-22

### Changed
- Bumped McHarbor and the remote agent to `1.6.2`.

### Fixed
- Fixed clipboard copy on plain HTTP non-loopback addresses (LAN installs, homelab IPs) silently reporting success while leaving the clipboard empty. Both `navigator.clipboard.writeText` and `document.execCommand('copy')` are unreliable in non-secure contexts and the previous helper trusted both, so the operator saw "copied to clipboard" but nothing was actually written. The clipboard helper now detects non-secure contexts up front and routes copy through a new `ManualCopyDialog` that renders the value in a pre-selected input with a hint to press Ctrl/Cmd+C. The dialog also offers a "Select & copy" button that re-attempts `document.execCommand` from a fresh user gesture. Applied to the agent token/install script/Docker command/binary command copy buttons in the agent token dialog, the API key copy button in `CreateAPIKeyDialog`, and the backup encryption key and setup command copy buttons in `StorageTab`.
- Added an explicit `isLocalhost()` helper to `resources/utils/clipboard.ts` that detects `localhost`, `127.0.0.1`, `127.0.0.0/8`, the IPv6 loopback `[::1]` / `::1`, and `*.localhost` subdomains. `isSecureClipboardContext()` now combines `window.isSecureContext` with the loopback check and the `https:` / `wss:` protocol check so it no longer treats a non-loopback secure-context page as copy-safe.

## [1.6.1] - 2026-06-21

### Changed
- Bumped McHarbor and the remote agent to `1.6.1`.

### Removed
- Removed the **System** menu from the sidebar. The application version, runtime info (platform, Go version), and dependency counts shown there were a subset of what **Settings → About** already provides (full backend and frontend dependency lists, plus update checking). The Settings → About tab is now the single place for application diagnostics. Removed the `/system` route, the `frontend/modules/system/` module (pages, components, hooks, types), the `system` i18n namespace and locale files, and the `nav.system` translation key. The footer version display now reads `/api/about` directly instead of the removed `useSystemInfo` hook. The backend `system` module is retained because its `/api/system/os-logs`, `/api/system/os-terminal/ws`, and `/api/system/os-updates/*` endpoints are used by the Environments module for host logs, host terminal, and host updates.

## [1.6.0] - 2026-06-16

### Added
- Added a first-class **Host** page (`/host`) with live CPU%, used memory, 1/5/15 minute load, host filesystem usage, Docker disk breakdown (images, containers, volumes, build cache), and a Prune panel with per-resource actions (system, builder, volumes, images, containers, networks) plus an explicit "System + volumes" destructive option. Live metrics come from `/proc/stat` + `/proc/meminfo` + `/proc/loadavg` and `statfs(2)` on Linux; the Docker SDK's `ImagesPrune` / `ContainersPrune` / `NetworksPrune` / `VolumesPrune` / `BuildCachePrune` are used individually to emulate `docker system prune` (the Go SDK does not expose it as a single call). `Volumes=true` is the explicit opt-in for system prune that also removes named volumes; the response is audit-logged and the host, containers, images, volumes, and networks queries are invalidated.
- Added per-resource RBAC: `host.view` and `host.manage` permissions gate `GET /metrics/host*` and `POST /metrics/host/prune` respectively. `agentLimit` is reported on the host response so the UI can show a clear state when the environment is connected through a remote agent (where live host metrics and prune are not available).
- Added an amber notice on the Host page when the current environment is agent-connected, so operators don't see a misleading "all zeros" state for a remote machine.
- Added **Auto-heal** (autoheal-by-RunTip42-style) for containers with a Docker healthcheck. Operators enable it per container from the new "Auto-heal" card in the container Settings tab. A background engine polls every 30s, identifies containers whose healthcheck reports `unhealthy`, and restarts them with exponential cooldown (30s → 1m → 2m → 5m). Two safety guards are mandatory: the first heal only happens after the container has been healthy at least once (no loops for misconfigured healthchecks), and the cooldown is enforced between restarts. Every heal is audit-logged and fires an in-app warning notification. Endpoints: `GET /api/autoheal/preference/{id}` and `POST /api/autoheal/preference/{id}` (body `{ enabled }`). Gated by the existing `containers.manage` permission. Engine starts/stops alongside the alerts engine in `main.go`.

## [1.5.5] - 2026-06-16

### Added
- Added **Used / Unused** visibility to the Networks page: each network card and the table `Containers` column now show a status badge (success = used, secondary = unused) and a new segmented **All / Used / Unused** filter (with live counts) drives both card and table views. The unused badge highlights networks that can be pruned.
- Added a **Remove network** option to the container remove dialog. When a user-defined network would become orphaned by the container removal (it has exactly that one container attached and is not `bridge`/`host`/`none`), the dialog now offers to remove it as part of the same operation, and reports per-network success and failure in the response.
- Added a full-options **bulk remove dialog** for multi-selected containers. Selecting several containers and clicking **Remove** now opens a dialog that mirrors the single-container options: Remove volumes, Remove image, Remove stack, and Remove network. The bulk loop runs each removal sequentially, sends the proper `force: true` request body, and lets `useRemoveContainer` keep the networks, containers, images, and stacks queries in sync.

### Fixed
- Fixed the network list endpoint always reporting `Containers: 0`: the Docker REST API `GET /networks` does not populate the `Containers` field (only `GET /networks/{id}` does), so the previous list response showed every network as unused. The networks service now cross-references a single `ContainerList` (`All: true`) call to compute an accurate per-network container count in the list response.
- Fixed bulk container delete silently doing nothing. `useContainerAction` was posting to `/containers/{id}/remove` with no body, but `HandleRemoveExtended` calls `response.DecodeBody` which fails on the empty request — so the request was rejected with `ErrInvalidBody` and no container was removed. Bulk removal now goes through the proper `useRemoveContainer` mutation (which posts the correct body with `force: true`).
- Fixed bulk network delete doing the same thing: the batch action's `onClick` loop called `setConfirmTarget(row.Id)` for every selected row, so only the last network survived and the single `ConfirmDialog` only removed that one. Bulk network removal now opens a dedicated `BulkRemoveNetworkDialog` that lists all selected networks (with in-use markers and a warning) and calls `useRemoveNetwork.mutateAsync(id)` per network sequentially; per-network failures are surfaced via the mutation's error toast and the loop continues.
- `useRemoveContainer` now also invalidates the `networks` query after success, so a removed-and-now-orphaned network disappears from the Networks page without a manual refresh.

## [1.5.4] - 2026-06-13

### Changed
- Bumped McHarbor and the remote agent to `1.5.4`.
- Migrated the project agent configuration from the legacy `.agents/` (Claude Code format) to the opencode-native `.opencode/` layout: 5 subagents now live at `.opencode/agent/<name>.md`, 19 skills at `.opencode/skills/<name>/SKILL.md`, 5 rules at `.opencode/rules/*.md`, and 4 legacy bash hooks at `.opencode/hooks/*.sh` (preserved for reference; their protection logic is now encoded in `opencode.json` `permission` rules).
- Wired the migrated configuration in `opencode.json`: `instructions` array now points at `AGENTS.md` plus the 5 rule files, `skills.paths` points at `.opencode/skills`, and the `permission` block carries the merged bash, read, edit, and webfetch rules previously held in `.agents/settings.json` and `.agents/settings.local.json`.
- Removed the legacy `.agents/` directory (CLAUDE.md, settings.json, settings.local.json, agents/, skills/, rules/, hooks/).

## [1.5.3] - 2026-06-12

### Changed
- Bumped McHarbor and the remote agent to `1.5.3`.

### Fixed
- Fixed App Store Compose generation for env-based list items so volume mounts such as Drupal, WordPress, Joomla, Ghost, Matomo, MediaWiki, Moodle, and Odoo render as valid quoted `host:container` strings.
- Added full bundled App Store Compose validation that renders every app template with defaults and verifies it with Docker Compose, preventing invalid generated YAML from reaching installs.
- Fixed stack API responses so stacks without discovered services return `services: []` instead of `services: null`, preventing the stacks page from crashing when an agent is unavailable or a stack has no live containers.
- Extended token-protected agent archive upload and download transfer deadlines so large agent-side backup and restore archives are not cut off by the default HTTP timeout.

## [1.5.2] - 2026-06-12

### Added
- Added agent-side container backups for agent environments using local agent temp archives: updated agents can now create encrypted backup archives beside the remote Docker socket and upload the completed archive to McHarbor local backup storage through a one-use direct HTTP transfer.
- Added agent-side full-archive restores for agent environments: updated agents can download the encrypted archive to local temp storage, decrypt it locally, and restore selected image, filesystem, or mount entries through the local Docker socket.
- Added internal one-use full-archive upload and download transfer endpoints for authenticated agents, separate from the existing single-entry restore transfer path.
- Added agent backup and restore progress events for local archive creation, upload, download, and restore apply phases.

### Changed
- Bumped McHarbor and the remote agent to `1.5.2`.
- Agent-backed backups now prefer the new agent-local archive path when the connected agent is `1.5.2+` and the selected backup destination is McHarbor local storage; older agents continue to use the existing fallback path.
- Restore on agent-backed environments now prefers full-archive agent restore when the connected agent is `1.5.2+`, reducing Docker-over-WebSocket transfer overhead for large filesystem and mount restores.

### Fixed
- Prevented active in-process backup and restore runs from being marked abandoned by polling recovery while they are still executing.
- Improved stalled backup diagnostics for long agent filesystem exports by moving supported backups off the generic Docker-over-WebSocket archive stream.

## [1.5.1] - 2026-06-10

### Changed
- Bumped the patch application version to `1.5.1` across canonical runtime metadata, agent metadata, frontend package metadata, and lockfile root metadata.
- Added version reporting for the local McHarbor instance and connected agents, including backend version APIs and centered version display on environment connection cards.
- Improved the container Backups tab layout by making manual and scheduled backup sections collapsible and grouping restore-from-file actions with the saved backup area.
- Added delete actions for completed and failed container backup runs, including local archive cleanup when an archive exists.
- Added restore item selection so users can choose which saved backup contents to restore, including images, container filesystems, and individual mounted data archives.
- Added pull-based agent transfer support for large restore and move payloads: agents can now pull one-use archive URLs from McHarbor, stage data locally, validate transfer size where available, and apply images or container archives through the local Docker socket.
- Added direct container archive transfer support between agents for move volume/path copies, with one-use target receivers and transfer progress reporting.
- Updated the frontend dependency lockfile for the current Vite/React lint and build toolchain.
- Raised Docker-based Go build images and backend module metadata to Go `1.26.4`.

### Fixed
- Fixed backup tab translation rendering so the Backups tab resolves to a string label instead of an object-valued translation key.
- Fixed frontend i18n validation for co-located workflow node and widget translation files, including ES/FR/PT fallback coverage for new backup, stack-link, and storage-location workflow nodes.
- Fixed React hook dependency warnings in backup, app store, network, container detail, and schedule input components.
- Fixed abandoned container backup runs staying in `running` after request cancellation, server timeout, shutdown, or agent stream interruption; stale runs are now finalized on startup/list, and timed-out backup streams are closed explicitly.
- Fixed manual container backups so a running entry appears in the Backups group immediately after clicking run, and the manual run button stays disabled until that backup finishes or fails.
- Added live backup progress stages under running backup timestamps, and use those progress heartbeats to fail stalled agent-stream backups instead of leaving them running.
- Added live restore progress for filesystem and mount restores, including byte progress while archive data is transferred to an agent or Docker target.
- Fixed large agent restore uploads hanging or being marked abandoned by moving filesystem and mount restores off the generic Docker-over-WebSocket upload path.
- Improved container move transfers to use the same staged transfer model as backup restores when moving images and named-volume data to agent-backed environments.
- Improved container backup failure diagnostics so missing backup encryption keys are logged per request and rejected before creating a failed run, while real run failures log the run ID, environment, container, stage, and internal cause.
- Fixed ignored encrypted credential decrypt errors in communication provider tests.
- Fixed container upload and restore error wrapping to preserve the original error cause.
- Fixed invalid persisted `agent_metrics_enabled` values so they no longer silently disable metrics.

## [1.5.0] - 2026-06-07

### Added
- Added reusable external storage locations for backups and exports, covering FTP, FTPS, SFTP, Samba, S3-compatible storage, Google Drive, OneDrive, and SharePoint, with encrypted credential storage and OAuth consent support for cloud providers.
- Added encrypted container backups with ad-hoc runs, saved backup plans, scheduled execution, selectable backup contents, backup run history, and archive downloads from the container detail Backups tab.
- Added container backup restore actions, including a secret-key prompt when an archive was encrypted with a different backup key than the current one.
- Added backup encryption key generation and one-click Docker secret installation from Settings, plus `docker-compose.secrets.yml` and backup key environment examples for manual setup.
- Added runtime backup encryption key status detection and an option to install a user-provided backup key from Settings.
- Added backup retention settings so administrators can cap successful container backups by maximum age and maximum count per container.
- Added workflow nodes for running container backups, running backup plans, downloading backup archives, listing and reading storage locations, and linking or unlinking containers from Compose stacks.
- Added HTTPS certificate management under Security, including certificate metadata, upload controls, and HTTPS/force-HTTPS toggles.
- Added OpenAPI coverage for storage locations, storage OAuth flows, backup encryption keys, and container backup plan/run endpoints.

### Changed
- Upgraded the frontend production toolchain to Vite `8.0.16` with `@vitejs/plugin-react` `6.0.2`, refreshed the frontend lockfile, and added compatibility aliases for the Vite 8/Rolldown build.
- Updated backend dependencies including `modernc.org/sqlite` `1.52.0`, newer `goja`, refreshed OpenAPI-related transitive packages, and updated Go module checksums.
- Scoped persisted dashboard layouts per selected environment so widget placement can differ between environments.
- Moved health/about endpoints onto explicit public route registration so runtime metadata remains public without sharing auth route behavior.
- Consolidated email and communication settings into a Communications tab and added a dedicated Storage tab for storage locations and backup encryption setup.
- Bumped the minor application version to `1.5.0` across canonical runtime metadata, agent metadata, frontend package metadata, and lockfile root metadata.

### Fixed
- Allowed protected McHarbor container runtime edits and renames only through an explicit unlock flow while keeping destructive protected-container actions blocked.
- Improved container header locking states so edit-only unlocks do not imply start, stop, restart, move, kill, or remove permissions.
- Improved shared data grid toolbars and select popovers so action controls and dropdowns behave better in dense settings and backup forms.
- Improved the generated backup key state with a green success indicator, explicit warning, and regenerate action.

## [1.4.1] - 2026-06-04

### Added
- Added editable Linux device mappings to the container Resources tab, allowing host devices such as `/dev/ttyUSB0` to be mapped into recreated containers with Docker `r/w/m` permissions.
- Added multi-file and folder upload support in the container file browser, including progress reporting and clearer upload failure messages.

### Changed
- Bumped the patch application version to `1.4.1` across canonical runtime metadata, agent metadata, frontend package metadata, and lockfile root metadata.
- Paused high-volume agent polling while container file uploads are active so large uploads do not compete with container lists, stats, host metrics, Docker info, update checks, or Docker event streams over the same agent connection.

### Fixed
- Fixed large folder uploads failing with plain `400` responses by streaming multipart upload parts to temporary files instead of parsing the full multipart form in memory.
- Fixed container upload archive creation to stream tar data to Docker instead of buffering the archive in memory.
- Fixed the upload dialog accessibility warning by providing the required dialog description.

## [1.4.0] - 2026-06-03

### Added
- Added agent-side Docker Compose execution for remote agent environments, allowing managed stacks and App Store installs to run full `docker compose` operations on the target agent host.
- Added `MCHARBOR_COMPOSE_DIR` to the agent runtime so Compose project files are staged on a stable host-mounted workspace.

### Changed
- Bumped the minor application version to `1.4.0` across runtime metadata, agent metadata, and frontend package metadata.
- Updated Docker-based agent images to include Docker CLI and Compose support.

## [1.3.7] - 2026-06-02

### Added
- Added token-safe target receiver diagnostics to the Settings > Agent direct transfer test popup, including whether the receiver existed, expired, matched the expected kind, received a bearer header, and matched the one-time token.

### Changed
- Bumped the patch application version to `1.3.7` across runtime metadata, agent metadata, frontend package metadata, generated agent install image references, README examples, and agent documentation.
- Raised the Settings direct-transfer diagnostic requirement to `mcharbor-agent` `1.3.7`; container moves still require `mcharbor-agent` `1.3.5+` for direct image transfer.

## [1.3.6] - 2026-06-02

### Added
- Added an agent-to-agent direct transfer connectivity test to Settings > Agent. The test prepares a one-use probe receiver on the target agent and asks the source agent to POST directly to it, reporting the phase, HTTP status, advertised URL, probe URL, duration, versions, and failure reason.
- Added a lightweight direct transfer probe endpoint to the remote agent so reachability and bearer-token validation can be tested without loading Docker images or changing containers.

### Changed
- Bumped the patch application version to `1.3.6` across canonical runtime metadata, agent metadata, frontend package metadata, lockfile root metadata, generated agent install image references, and README agent image examples.
- Generated agent install and deploy commands now use `ghcr.io/therealmcsparrow/mcharbor-agent:1.3.6`.

## [1.3.5] - 2026-06-01

### Added
- Added optional direct agent-to-agent container snapshot image transfer for moves between remote agent environments. When both agents are updated and the target agent advertises a transfer listener, snapshot data streams from source agent to target agent instead of through the McHarbor server.
- Added target-agent transfer listener configuration through `MCHARBOR_TRANSFER_LISTEN` and `MCHARBOR_TRANSFER_ADVERTISE_URL`.

### Changed
- Bumped the patch application version to `1.3.5` across canonical runtime metadata, agent metadata, frontend package metadata, lockfile root metadata, and generated agent install image references.
- Raised the direct-transfer agent requirement to `mcharbor-agent` `1.3.5`; moves automatically fall back to the existing McHarbor relay path when direct transfer is unavailable.

### Fixed
- Fixed connected remote-agent container updates leaving the old agent container running by passing the retired container ID to the replacement agent so it can remove the old container from the host Docker socket after startup.
- Fixed remote-agent image load hangs by streaming Docker `/images/load` request bodies through the agent even when the Docker SDK reports a zero content length.
- Added move cancellation UI handling and de-duplicated repeated image-load progress messages.
- Added editable move volume mapping for both named volumes and bind mounts, including target host path and target container path.

## [1.3.4] - 2026-06-01

### Changed
- Added editable target volume mount settings to container moves, prefilled from the source volume name and container path.
- Pinned generated remote agent Docker install commands to the current agent version and force-pull before restart so stale local `latest` images are not reused.
- Added progress of receiving staged image on target docker.
- Bumped the patch application version to `1.3.4` across canonical runtime metadata, agent metadata, frontend package metadata, lockfile root metadata, and generated agent install image references.

## [1.3.3] - 2026-06-01

### Changed

- Bumped the patch application version to `1.3.3` across canonical runtime metadata, agent metadata, frontend package metadata, and lockfile root metadata.
- Raised the target-agent requirement for container snapshot/image data moves to `mcharbor-agent` `1.3.3` so move operations use the staged image-upload protocol.


### Fixed

- Fixed remote-agent container moves still stalling after the first target image-load chunk by staging `/images/load` uploads on the agent before calling the local Docker daemon.
- Fixed duplicate same-version agent connections replacing the active connection during long container moves.
- Fixed rejected older duplicate agents overwriting the connected agent version shown in environment status.

## [1.3.2] - 2026-05-31

### Changed

- Bumped the patch application version to `1.3.2` across canonical runtime metadata, agent metadata, frontend package metadata, and lockfile root metadata.
- Extended the bounded container move operation window so remote agent image and volume moves can finish on slower Docker hosts.
- Changed container moves to snapshot and transfer the source container filesystem layer, preserving data written inside containers even when they do not use Docker volumes.
- Switched container image loading during moves away from quiet Docker load mode so target Docker can stream load progress/results back through the agent path instead of waiting until unpacking completes.
- Raised the target-agent requirement for container snapshot/image data moves to `mcharbor-agent` `1.3.2` so older agents fail fast instead of hanging during upload.
- Changed agent registration to keep the newest connected agent for an environment, preventing an older duplicate agent process from replacing the active connection during long transfers.

### Fixed

- Fixed moved containers missing Docker volume attachments when the original container used image-declared or otherwise implicit named/anonymous volumes.
- Fixed moved containers without volumes being recreated from the original image without their writable-layer data.
- Fixed container moves timing out while the target Docker daemon was still loading the transferred image archive.
- Fixed the remote agent streamed request-body reader used by Docker image loads so large snapshot archives do not stall after the first chunk.
- Fixed long container moves failing with `agent transport closed` when duplicate old and new agents were running with the same environment token.

## [1.3.1] - 2026-05-31
### Added

- Added container moves between Docker environments, including a preview dialog that lists required image, volume, stack-label, and network changes before execution.
- Added editable target network settings to the container move dialog, including target network name, network mode, driver/type, IPAM subnet/gateway/range, aliases, target IP/MAC, and internal/attachable options before moving the container.

### Changed

- Added move execution support for transferring missing images, creating missing named volumes and Docker networks, copying named volume data, preserving Compose stack labels, and optionally stopping/removing the source container.
- Kept the production frontend build on Rollup-based Vite `7.3.3` with `@vitejs/plugin-react` `5.2.0` to avoid the Vite 8 Rolldown Recharts chunk runtime regression.
- Bumped the patch application version to `1.3.1` across canonical runtime metadata, agent metadata, frontend package metadata, and lockfile root metadata.

### Fixed

- Cleared frontend packages reported by `npm outdated` where compatible, while holding Vite and `@vitejs/plugin-react` on their Rollup-based major versions because Vite 8 generated a broken Recharts production chunk.
- Fixed the production chart bundle regression that caused Recharts Cartesian chart chunks to throw `TypeError: t is not a function` at runtime.
- Fixed ineffective cancellation breaks in agent Docker proxy streaming loops so canceled HTTP streams and exec sessions exit the intended loop.
- Prevented Recharts dashboard charts from rendering before their containers have a positive measured size, avoiding repeated `width(-1)` / `height(-1)` console warnings during dashboard layout changes.

### Tests

- Ran the frontend validation suite after pinning the production build back to Vite 7.
- Ran the agent validation suite after the proxy cancellation fix.
- Ran the frontend validation suite after adding measured chart containers.
- Ran the backend version package validation and backend i18n coverage after the minor version bump (`.results/tests/backend-20260531-040651.log`).
- Ran the agent validation suite after updating agent metadata (`.results/tests/agent-20260531-040651.log`).
- Ran the frontend validation suite after updating frontend package metadata (`.results/tests/frontend-20260531-040657.log`).
- Rebuilt and restarted the Docker container and verified `/api/health` and `/api/about` after the minor version bump.
- Ran the backend containers validation and backend i18n coverage after adding editable move network settings (`.results/tests/backend-20260531-042302.log`).
- Ran the agent validation suite after updating agent version metadata (`.results/tests/agent-20260531-042302.log`).
- Ran the frontend validation suite after adding the move network settings UI and updating package metadata (`.results/tests/frontend-20260531-042302.log`).

## [1.2.4] - 2026-05-30

### Changed

- Updated frontend dependency lockfile packages to the newest versions allowed by the existing semver ranges, including Vite `8.0.14`, React `19.2.6`, React Router `7.16.0`, Tailwind CSS `4.3.0`, TanStack Query `5.100.14`, i18next `26.3.0`, and related tooling/runtime packages.
- Updated backend Go dependencies, including chi `5.3.0`, Kubernetes client libraries `0.36.1`, modernc SQLite `1.51.0`, `golang.org/x/crypto` `0.52.0`, `golang.org/x/net` `0.55.0`, SAML XML signature support `1.6.0`, and related transitive modules.
- Bumped the backend Go module directive to `go 1.26.0`, matching the Docker builder image already used for production builds.
- Bumped the canonical application version in `VERSION`, agent metadata, frontend package metadata, and lockfile root version to `1.2.4`.

### Fixed

- Cleared all frontend packages reported by `npm outdated` within the configured package ranges.
- Refreshed backend module checksums after the Go dependency update and tidy pass.

### Tests

- Rebuilt the frontend successfully after the npm update.
- Ran the full backend test suite after Go dependency updates.
- Rebuilt and restarted the Docker container and verified `/api/health` returned OK after the dependency updates.

## [1.2.3] - 2026-05-30

### Added

- Added container renaming across the backend API and containers UI, including validation, translated feedback, audit logging, and protected-container safeguards.
- Added workflow nodes for Docker event triggers, container health triggers, registry tag triggers, environment status checks, approval gates, image vulnerability scans, stack backups, and Docker volume backup/restore operations.
- Added automatic workflow trigger handling for Docker events, container health changes, and watched registry tag digest changes.
- Added localized workflow node metadata for English, Dutch, German, Spanish, French, and Portuguese.
- Added a file-based bundled app catalog under `apps/`, with expanded templates for popular web apps and WhatsApp gateway deployments.

### Changed

- Moved bundled workflow node definitions and dashboard widgets to root-level `nodes/` and `widgets/` catalogs, with Vite/TypeScript aliases resolving them from the frontend build.
- Changed the app store catalog loader to read individual bundled app JSON files from the runtime `apps/` directory instead of one embedded catalog file.
- Improved container recreate behavior by omitting runtime-only network endpoint fields, preserving create-time endpoint options, and handling connected agent containers without tearing down the active agent connection too early.
- Bumped the canonical application version in `VERSION`, agent metadata, frontend package metadata, and lockfile root version to `1.2.3`.

### Fixed

- Fixed agent reconnect cleanup so an older WebSocket disconnect does not remove a newer active agent connection.
- Fixed workflow node palette visibility so unavailable nodes stay hidden by default.
- Fixed app store Compose generation coverage for database-backed templates and quoted bind mount rendering.

### Tests

- Added app store catalog coverage for bundled web apps, WhatsApp gateway templates, duplicate slugs, and generated Compose output.
- Added container rename validation and recreate networking regression coverage.
- Added workflow node regression coverage for Docker event matching, container health parsing, approval routing, and backup path validation.

## [1.2.2] - 2026-05-29

### Added

- Added protected-resource detection for the running McHarbor container, including support for the `com.mcharbor.protected` label.
- Added protected state to Docker container API responses so the UI can lock unsafe self-management actions against the running McHarbor container.
- Added UI lock handling that prevents container bulk actions, remove actions, update/reinstall actions, and file edits from targeting the running McHarbor container.
- Added a GitHub repository shortcut to the About page and copy controls for both Docker and binary remote-agent install commands.
- Added per-workflow export and import for portable workflow JSON files.

### Fixed

- Fixed destructive container operations so McHarbor no longer permits deleting, pruning, stopping, restarting, pausing, killing, updating, recreating, mutating files in, or disconnecting the running McHarbor container.
- Fixed container prune behavior to enumerate candidates explicitly and skip the protected McHarbor container instead of relying on broad Docker prune calls.
- Fixed update-check failures so unreachable GitHub responses return a translated error state instead of falsely reporting the current install as up to date.
- Fixed remote-agent SSH deployment requests so Docker-based installs pass the intended agent image through the deployment payload.
- Fixed backend hardening gaps around generated credentials, role updates, encryption setup, and middleware timing/error paths.

### Changed

- Replaced special-case self-recreate handling with shared protected-resource guards for container action paths while keeping images, stacks, and volumes mutable.
- Removed Chinese from the supported interface languages and locale bundles.
- Consolidated application version metadata around the root `VERSION` file so runtime version displays, update checks, OpenAPI metadata, and image publishing use one source.
- Bumped the canonical application version in `VERSION` to `1.2.2`.

### Tests

- Added regression coverage for protected McHarbor container detection.
- Added workflow import and export handler coverage.
- Expanded i18n and health/about test seams to cover the updated update-check and about metadata paths.

## [1.2.1] - 2026-05-27

### Added

- Added the System menu with overview metrics, services, processes, dependencies, OS terminal, OS logs, and OS update tabs.
- Added protected host OS system endpoints for terminal sessions, bounded log snapshots, and package update check/apply actions.
- Added System page, OS update flow, and OS log notice translations for every supported UI language.

### Fixed

- Fixed deprecated Compose naming risk by removing fixed `container_name` entries from production and development Compose files so project-scoped container names can be generated safely.
- Fixed stack Docker Compose subprocess handling by adding bounded contexts and switching calls to context-aware command execution.
- Fixed McHarbor self-update scheduling module boundaries by moving detached helper scheduling into the shared Docker core package and removing the containers module dependency on the stacks module.
- Fixed agent terminal resize requests so HTTP responses are closed and drained instead of being ignored.
- Fixed Docker event stream EOF handling so expected disconnects are treated as reconnectable stream closes instead of noisy backend errors.
- Fixed OS log collection on hosts where `journalctl` is unavailable or not readable by preferring direct host log files first, falling back only when needed, and surfacing non-fatal permission/source notices in the UI.
- Fixed missing System navigation and namespace translations for Spanish, French, Portuguese, and Chinese.
- Fixed several sanity-check warnings around ignored Go test write errors, frontend non-null assertions, unsafe DataGrid filter typing, theme-token hover styling, and destructive role-delete styling.

### Changed

- Added a deprecated-usage sanity check that reports deprecated APIs, packages, Docker/Compose syntax, and migration suggestions.
- Kept OS log collection read-only and unprivileged while guarding host terminal and update actions behind dedicated permissions.
- Logged the received shutdown signal during graceful backend shutdown for easier production diagnosis.
- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.2.1`.

### Tests

- Added regression coverage for the shared self-update scheduler mount fallback.

## [1.2.0] - 2026-05-24

### Added

- Added an environments card overview with a persistent table/card switch, CPU and RAM sparklines, container state totals, and per-environment image update counts.
- Added local-user profile management, including account details editing for display name and email, a dedicated profile page, profile navigation from the avatar menu, and localized profile copy.
- Added authenticated user language preferences with session/login payload support, protected preference endpoints, and persisted profile/settings language selectors.
- Added Spanish, French, Portuguese, and Chinese as selectable interface languages, including frontend resource bundles, widget/node translations, backend API message translations, and language negotiation.
- Added security user creation for local accounts, including backend validation, duplicate username handling, default role assignment, RBAC cache invalidation, audit logging, frontend creation controls, and localized copy.
- Added manual container-to-stack relinking from container detail and stack detail views, backed by a persisted link table.
- Added configured delivery and registry selectors across workflow communication, email, webhook, image pull, image push, and registry search nodes, including saved channel/webhook/registry pickers, custom fallback modes, HMAC-signed configured webhooks, custom SMTP settings, encrypted credential reuse, and reusable config field renderers.

### Changed

- Matched environment overview cards to container card grid widths across responsive breakpoints.
- Moved theme and language controls from the avatar menu into the profile page.
- Made prune-unused actions visible as page-level actions for containers, images, volumes, and stacks.
- Expanded i18n validation to cover every locale directory plus co-located widget and workflow node translation files.
- Updated the README AI section to describe AI-assisted translation, review, documentation, and developer workflow support.
- Updated OpenAPI documentation for manual container-to-stack link, relink, and unlink endpoints.
- Improved workflow node requirement inference so conditionally hidden fields do not appear required when inactive.
- Hardened auth preference and session update paths by surfacing database write and lookup errors instead of silently ignoring them.

### Fixed

- Fixed cron schedule previews so valid schedules with timezone labels no longer render as “Invalid cron expression”.
- Fixed container take-over so adopting a standalone container creates a durable container-to-stack link instead of only creating a managed stack record.
- Preserved selected container rows across polling refreshes by pruning table selection only when selected row IDs disappear.

### Tests

- Added backend i18n tests for the expanded language negotiation and message registrations.

## [1.1.15] - 2026-05-24

### Changed

- Added Piper and Speech-to-Phrase to the bundled App Store catalog.
- Added Frigate, Homebridge, go2rtc, Scrypted, InfluxDB, EMQX, EVCC, rtl_433, Ring-MQTT, and AppDaemon to the bundled App Store catalog.
- Updated Technitium's catalog display name to Technitium DNS Server.
- Enabled App Store compose overrides for entries that require a custom command block.
- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.15`.

## [1.1.13] - 2026-05-24

### Fixed

- Fixed container-page update and reinstall actions for the running McHarbor container by routing `POST /api/containers/{id}/recreate` through the detached self-update helper instead of stopping the API process inline.
- Fixed the production helper preparation path when `DATA_DIR` is unset or relative by matching the actual container mount destination `/app/data`.
- Kept the container update progress recovery logic active even when the backend successfully schedules a self-update helper before the API restarts.

### Tests

- Added regression coverage for detecting the McHarbor app container while excluding the remote agent image.
- Added regression coverage for the `/app/data` helper mount fallback used by Docker-run production deployments.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.13`.

## [1.1.12] - 2026-05-24

### Fixed

- Fixed self-update detection for McHarbor containers that were started without Docker Compose labels and only expose stale OCI image labels.
- Matched the conventional running container name `mcharbor` against the managed stack name before falling back to plain compose operations.
- Added stored compose recognition for McHarbor image references that use environment-substituted tags such as `${MCHARBOR_TAG:-...}`.
- Fixed published OCI image metadata so `org.opencontainers.image.version` is explicitly set from the release version for both the McHarbor and agent images.

### Tests

- Added regression coverage for name-based self-container matching, McHarbor compose image recognition, and the Docker-name inspect fallback.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.12`.

## [1.1.11] - 2026-05-24

### Fixed

- Fixed self-update detection on Ubuntu hosts where cgroup v2 and `/proc` metadata may not expose the running container ID to McHarbor.
- Added a direct Docker inspect fallback for the conventional `mcharbor` container name before deciding whether a managed stack contains the running McHarbor instance.
- Prevented the self-update helper and watchdog path from being skipped when the running container can be inspected by Docker name but not by `/proc`-derived container ID candidates.

### Tests

- Added regression coverage to ensure the default `mcharbor` container-name fallback is always included in self-inspection candidates.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.11`.

## [1.1.10] - 2026-05-24

### Fixed

- Added McHarbor self-update/self-reinstall detection when a production managed stack name does not exactly match Docker Compose's `com.docker.compose.project` label.
- Added self-target detection using the running McHarbor container's compose project label, compose working directory, compose config-file label, stored compose `container_name`, and McHarbor image reference before allowing a managed stack update to use plain `docker compose`.
- Prevented self-updates from silently falling back to plain compose operations when the managed stack contains the running McHarbor container but Docker labels or adopted stack metadata differ.

### Tests

- Added regression coverage for self-container matching by compose working directory and stored compose `container_name`.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.10`.

## [1.1.9] - 2026-05-23

### Fixed

- Added a detached self-start watchdog for McHarbor self-update and self-reinstall operations so production Linux hosts can recover if Docker creates the replacement container but leaves it stopped.
- The watchdog starts before the destructive self-update step, ignores the old container ID, waits for the replacement container with the same McHarbor name, and repeatedly starts it until Docker reports it running.
- Added separate watchdog logs under `/app/data/self-update` so stopped-replacement cases show whether the replacement was missing, created, exited, or successfully started by the watchdog.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.9`.

## [1.1.8] - 2026-05-23

### Fixed

- Fixed the remaining production self-update and self-reinstall restart gap where Docker could recreate the McHarbor container but leave it stopped until an operator manually started it.
- Added repeated replacement-container start attempts in the self-update helper to tolerate Docker daemon cleanup timing on production Linux hosts after the old container is stopped and removed.
- Added post-start verification so the helper only marks the update complete after the replacement McHarbor container stays running briefly.
- Added detailed helper logging for failed start attempts and final replacement-container state to make future production restart failures diagnosable from `/app/data/self-update`.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.8`.

## [1.1.7] - 2026-05-23

### Fixed

- Reworked McHarbor self-update and self-reinstall in production so the running container is recreated directly through the Docker socket instead of relying on the stored managed-stack compose file.
- Fixed the failure mode where stale adopted compose metadata, including old image tags or generated container hostnames, could stop McHarbor and fail to bring it back online.
- Added a dedicated `self-update-helper` runtime mode that safely clones the current container configuration, pulls the target image for update operations, recreates the named McHarbor container, and rolls back to the original image if replacement startup fails.
- Preserved existing container mounts, ports, restart policy, labels, environment, and network attachment while filtering generated container-ID aliases that can poison future self-detection.

### Tests

- Added regression coverage for cloning self-container configuration, replacing the image tag, clearing generated hostnames, disabling auto-remove on the replacement container, and filtering generated network aliases.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.7`.

## [1.1.6] - 2026-05-23

### Fixed

- Hardened the production self-update and self-reinstall helper so McHarbor writes durable helper logs under `/app/data/self-update` before recreating itself.
- Added helper recovery behavior that retries `docker compose up -d` and then attempts to start the previous McHarbor container if the compose update path fails mid-flight.
- Fixed helper working-directory handling for managed stacks stored with relative project paths by normalizing them to `/app/...` inside the detached helper container.

### Tests

- Added regression coverage for self-update helper script generation, durable logging, recovery commands, and relative stack path normalization.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.6`.

## [1.1.5] - 2026-05-23

### Fixed

- Fixed production self-update and self-reinstall flows so McHarbor reliably detects when a managed stack contains the running McHarbor container, even after an adopted compose file pinned an old container hostname.
- Fixed the detached self-update helper to inspect the current container by real container ID candidates from `/proc` metadata before falling back to hostname-based lookup.
- Fixed compose reconstruction so Docker-generated container hostnames are no longer written into adopted compose files, preventing future self-update detection from being poisoned by stale container IDs.

### Tests

- Added regression coverage for reconstructed compose output and self-container matching with stale hostnames.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, and lockfile root version to `1.1.5`.

## [1.1.4] - 2026-05-23

### Fixed

- Fixed the operation progress dialog so the shared log header and empty-state copy are translated correctly instead of rendering missing-key identifiers.
- Fixed stack self-update and self-reinstall recovery in the frontend so McHarbor now waits for its API to come back online after recreating its own stack instead of failing the batch flow prematurely.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, README release reference, and frontend package version metadata to `1.1.4`.

## [1.1.3] - 2026-05-23

### Fixed

- Fixed managed stack self-update and self-reinstall flows so McHarbor can recreate its own stack without killing the in-flight `docker compose` command before the API comes back online.
- Fixed stack and container batch progress logs showing raw i18n keys by adding the missing shared `common.operations.log.*` translations for English, Dutch, and German.
- Fixed the self-restart recovery status text in container update flows so the progress dialog uses translated copy instead of hardcoded English.
- Fixed communication-channel test responses so missing channels no longer surface as generic `500` errors and now return a proper not-found response.
- Fixed Telegram test-channel error handling so Telegram admin-rights failures are mapped to a specific user-facing validation message instead of an opaque internal server error.

### Changed

- Bumped the McHarbor Docker image, agent image references, runtime metadata, README release reference, and frontend package version metadata to `1.1.3`.

## [1.1.2] - 2026-05-23

### Added

- Added a dedicated confirmation popup for the `Reinstall All` stack action so bulk reinstalls now require explicit confirmation before force-recreating managed stacks.
- Added localized confirmation copy for the new reinstall-all dialog in English, Dutch, and German.

### Changed

- Reused the shared confirmation dialog pattern for stack reinstall flows so the new modal behaves consistently with existing destructive confirmations such as prune actions.
- Bumped the McHarbor Docker image, agent image references, runtime metadata, and frontend package version metadata to `1.1.2`.

## [1.1.1] - 2026-05-23

### Changed

- Aligned the production Docker image tag defaults, optional agent image tag defaults, backend health/version metadata, update-check version reporting, frontend package version, and footer version display on `1.1.1`.
- Updated the README release reference and published the corresponding changelog entry so the documented default deployment version matched the shipped container images.

## [1.1.0] - 2026-05-23

Baseline: changes since `731ed97` (`Publice release`).

### Added

- Added generic OIDC authentication support, including backend provider handling, migrations, OpenAPI coverage, and frontend provider configuration screens (`92483e6`).
- Added SAML 2.0 authentication support, including identity-provider persistence, backend SAML helpers, and frontend configuration flows (`92483e6`).
- Added richer workflow help and discovery surfaces by splitting node documentation, general help, category sections, and shared palette icon utilities into dedicated frontend modules (`9e108a0`).
- Added a README screencast preview using `images/McHarbor.mp4` to make the current UI easier to inspect before deployment (`7abd0a3`).
- Added the standard McHarbor copyright header to terminal and cron preview components.
- Added deterministic agent token hashing and the `038_agent_token_hash.sql` migration so agent authentication can use indexed lookups instead of decrypting every stored token.
- Added extracted frontend modules to bring large screens and tabs back within the project’s component-size guideline:
  - `EditFieldControls.tsx`
  - `EnvironmentSections.tsx`
  - `ResourcesSections.tsx`
  - `ScannerToggle.tsx`
  - `StacksPageHeaderActions.tsx`
  - `useStackBatchOperations.ts`

### Changed

- Standardized frontend time formatting to use shared formatting utilities instead of ad hoc `toLocaleTimeString(...)` calls (`8660bbd`).
- Moved a shared dependency into a neutral shared layer and continued the same refactor direction in follow-up UI consistency work (`1f27c4b`).
- Split oversized frontend modules such as workflow help, node palette, files tab, network tab, stacks page, environment tab, resources tab, and scanners tab into smaller focused units (`1f27c4b`, `9e108a0`, local release hardening).
- Reworked app, stack, workflow, dashboard, and container UI controls to use shared `Button` and `Switch` primitives consistently instead of raw controls or ad hoc patterns.
- Normalized modal and search overlays to theme-backed backdrop styles across shared resources and overview dialogs.
- Replaced hardcoded workflow canvas colors and several inline layout styles with semantic theme tokens and stable utility classes.
- Standardized backend module route mounting for `appstore`, `custom_nodes`, and `workflows` so they expose the repo’s expected `Mount(app *router.AppDeps)` entrypoint while preserving explicit runtime injection paths.
- Renamed backend package declarations that used underscore names to Go-standard package names:
  - `api_keys` -> `apikeys`
  - `custom_nodes` -> `customnodes`
  - `docker_info` -> `dockerinfo`
  - `in_app_notifications` -> `inappnotifications`
  - `k8s_services` -> `k8sservices`
- Bumped the McHarbor application version from `1.0.0` to `1.1.0` across frontend packaging, runtime health/about metadata, update checks, startup logging, and OpenAPI metadata.

### Fixed

- Fixed duplicate ports rendering in the container tile view (`c3987b8`).
- Fixed the top-bar search layout issue reported by users (`4175e46`).
- Fixed several non-null assertion problems in the frontend codebase (`f3aa961`).
- Fixed operator-facing error handling so internal error details are sanitized before reaching the UI (`ed8108e`).
- Fixed a dynamic SQL `whereClause` splice issue in scan-related backend code (`ed8108e`).
- Fixed request-adjacent code that used `context.Background()` instead of deriving from the request context (`598c145`).
- Fixed non-standard initialism casing in exported backend types (`598c145`).
- Fixed remaining UI consistency failures from the sanity pass by aligning header actions, tab controls, widget controls, selection lists, and workflow config toggles to shared UI primitives.
- Fixed remaining React standards failures by shrinking components that exceeded the 200-line limit and extracting reusable pieces.

### Security

- Added indexed agent token validation backed by deterministic hashes, with legacy fallback and hash backfill for existing encrypted tokens.
- Stopped accepting workflow webhook secrets through query parameters; webhook validation now avoids the leaked-secret-by-URL pattern.
- Removed an unmanaged goroutine from API key middleware and replaced it with an inline, timeout-bounded `last_used_at` update path.
- Tightened route and package wiring around custom nodes and workflows while preserving explicit dependency flow.

### Documentation

- Updated the README with the embedded product screencast and direct video link (`7abd0a3`).
- Added this release changelog so the project has an explicit release history starting from the current public baseline.

### Internal

- Updated backend bootstrap wiring and imports to reflect standardized module mounts and renamed package declarations.
- Regenerated the frontend lockfile version metadata to match the release bump.
