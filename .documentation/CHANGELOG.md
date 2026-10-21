# Documentation Changelog

Changes to the documentation set in `.documentation/`. Pages carry no date stamps — what
changed, and which McHarbor version it was verified against, is recorded here.

Entries are reverse-chronological, and group their lines under **Added**, **Changed** and
**Removed** — omitting a group that is empty.

Every entry is stamped with the **date and the local 24-hour time** it was written, so two
changes made on one day stay distinguishable and in order. Entries written before this
convention carry a date only.

## 2026-10-21 14:06 — McHarbor 2026.10.01 — Kubernetes credential rules

### Changed

- `kubernetes/configuration.md` — **Cluster credentials**: the kubeconfig fields refused on save
  and when stored, the connection checks (`/version`, refused link-local and metadata
  addresses, the five-minute check), and how to convert a kubeconfig to inline data.
- `kubernetes/troubleshooting.md` — symptom rows for a refused save, `credentials_refused`,
  `api_unverified` and a metadata address; a **Reason codes** table with the dashboard titles.
- `kubernetes/README.md` — the refusal in **How it works**; the plugin version and host range
  now read `2026.10.01` and `>=2026.10.01`, matching `plugin.yaml`.

## 2026-10-16 15:29 — McHarbor 2026.10.01 — Frontend lint and format on Vite+

### Added

- `development/frontend/tooling.md` — the Vite+ lint and format setup: the commands, where the
  Oxlint and oxfmt configuration lives, the rule set, exceptions and their directive form, the
  out-of-tree trees, the hooks and CI step, and what goes wrong.

### Changed

- `development/frontend/README.md` — a **Lint and format** row in the page index; the component
  size limit is 300 lines; step 10 adds `npm run format:check`.
- `development/frontend/components.md` — **What the linter enforces** names Oxlint and
  `lint/config.ts` instead of `eslint.config.js`, the 300-line limit, the React Compiler,
  `jsx-a11y` and floating-promise rules, and the `mcharbor/no-restricted-syntax` directive form.
- `development/frontend/layout.md` — module boundaries are enforced by
  `mcharbor/module-boundaries` and `no-restricted-imports`, not ESLint's `boundaries`.

## 2026-10-12 13:17 — McHarbor 2026.10.01 — Agent builds kept, backup relink on recreate, network Edit

### Changed

- `agents/releases/README.md` — **Builds**: each build is stored on its own; a download you start
  becomes the target and keeps the previous one; a failed download marks only its own row. The
  table, its actions (now naming the build), **Releases to keep**, the routes' `?build=` /
  `build` parameters, the CLI examples and three troubleshooting rows follow.
- `settings/updates/agents/README.md` — the **Agent releases** bullet: one row per build, the
  download sets the target, the agents table re-checks on a target change.
- `settings/updates/agents/reference.md` — **Releases to keep** counts builds; the target is set
  per build; a row for going back to the previous build.
- `cli/README.md` — the Agent releases row: `--build` on `target` and `delete`, and `pull` making
  the download the target.
- `api/endpoints/backups.md` — plans and runs follow a recreated container on Docker events;
  `relink-all` is the fallback for environments without **Track container events**.
- `backup/screen/troubleshooting.md` — a row for backups that lose their container after a
  recreate.
- `screens/networks/actions.md` — an **Edit** row.

## 2026-10-07 00:50 — McHarbor 2026.10.01 — Adding an app to the catalog

### Added

- `how-to/apps/README.md` — adding an app to the Store catalog: the spec fields, compose
  template placeholders (and the one-placeholder rule for list items), the generator, the index
  build and its limits, publishing to `https://www.mcharbor.eu/catalog/apps/`, with `hass-otbr`
  as the worked example.

### Changed

- `how-to/README.md` — the new guide in the Guides table.
- `screens/store/troubleshooting.md` — a row for an app with host networking or a device that
  installs but does not work, because McHarbor does not prepare the host.

## 2026-10-06 20:40 — McHarbor 2026.10.01 — Configuration restore, folder restores, healthcheck Test

### Added

- `backup/screen/actions.md` — **Restoring configuration**: the recreate, the dialog's warning,
  and the two-pass restore when the filesystem layer is restored too.
- `screens/containers/actions.md` — a **Test healthcheck** row.

### Changed

- `backup/screen/actions.md` — a ticked folder restores everything beneath it; what
  **Restored items** marks as restored; **Restore from file**'s note on configuration now points
  at the new section.
- `backup/screen/fields.md` — a restore run's size reads "—"; the name is the section's first field.
- `screens/containers/fields.md` — the healthcheck **Test** button under Detail-tab fields.
- `api/endpoints/backups.md` — the restore route's configuration and filesystem behaviour.
- `how-to/plugin/packaging.md` — when the Plugin Store offers **Update**, for local and remote
  catalogs.

## 2026-10-06 03:52 — McHarbor 2026.10.01 — Vault secrets in the VM wizard

### Added

- `development/contracts/host-services.md` — the `Secrets` host handle: `Resolve`, `Release`, the
  `SecretCode()` refusals, and the `secrets.read` token that hands it out.

### Changed

- `virtual-machines/README.md` — the wizard's password and SSH keys from the vault, the
  permission it needs, the one-time copy, and the CLI flags.
- `api/endpoints/secrets.md` — a plugin's use of a secret appears in the impact report as
  **Copied once**.

## 2026-10-06 03:29 — McHarbor 2026.10.01 — Restore from file, agent builds, cloud images

### Added

- `backup/screen/actions.md` — a **Restore from file** section: the two steps (**Read backup**,
  then the usual choices), what a wrong key does, and moving a container to another server.
- `agents/releases/README.md` — **Pinned builds**: what pins a build, what unpins it, and that
  a named build is fetched from its own folder only.
- `compute-foundation/image-catalog.md` — **Publishing the official catalog**: the
  `mnt/compute-catalog/` staging folder and `scripts/compute/build-image-index.mjs`.
- `virtual-machines/troubleshooting.md` — **Checking the cloud-init seed**, plus rows for a
  rejected wizard login and for "never opened its monitor socket".

### Changed

- `api/endpoints/backups.md` — the new `POST /api/containers/{id}/backups/upload`; `restore-upload`
  is described as the one-shot full restore it now is.
- `agents/releases/README.md` — **Check now** keeps the last five builds per version; the pull
  picker, the CLI `--build` flag and `?build=` on the pull route.
- `settings/updates/agents/reference.md` — a row for a release that does not follow a newer build.
- `compute-foundation/image-catalog.md` — the two image sources, with the official one at
  `https://www.mcharbor.eu/catalog/images/index.json`.
- `compute-foundation/host-requirements.md`, `virtual-machines/spice-console/README.md` — SPICE
  is detected by starting QEMU with it; `qemu-system-modules-spice` on Debian and Ubuntu.
- `virtual-machines/README.md` — how the cloud-init seed is delivered, and the `adm`-only account.

## 2026-10-05 20:06 — McHarbor 2026.10.01 — Go crashes on AVX-512 CPUs

### Changed
- `getting-started/installation/native-linux.md` — Troubleshooting item 4: heap-corruption crashes
  on AVX-512 CPUs, the `GODEBUG` setting McHarbor and the agent now start with, the Docker and
  containerd drop-ins, and how to verify them.
- `agents/troubleshooting/README.md` — **Crashes on AVX-512 CPUs** section.

## 2026-10-05 10:01 — McHarbor 2026.10.01 — Install compute packages from the capability panel

### Added
- `compute-foundation/host-requirements.md` — **Installing from the capability panel**: which rows get
  an Install button, the package each means per distribution, the permission, audit, route, CLI
  and agent floor.

### Changed
- `compute-foundation/host-detail.md` — the capability panel points at the Install button.

## 2026-10-05 09:36 — McHarbor 2026.10.01 — Host log order and live refresh

### Changed
- `screens/environments/README.md` — the Console tab's host log names its order toggle and **Live** refresh.

## 2026-10-05 08:46 — McHarbor 2026.10.01 — Native runs as root by default

### Changed
- `getting-started/installation/native-linux.md` — the service runs as root by default; section
  7 is rewritten around it, **Running as the mcharbor account** replaces **Running as root, if
  you choose to**, and Troubleshooting item 3 applies to `--no-run-as-root` installs.
- `getting-started/prerequisites/README.md` — the native paragraph describes both accounts.
- `settings/integrations/storage/reference.md` — the read-only row applies to
  `--no-run-as-root` installs.

## 2026-10-05 08:18 — McHarbor 2026.10.01 — Scanner binaries and running as root

### Added
- `getting-started/installation/native-linux.md` — **Running as root, if you choose to**:
  `--run-as-root` / `MCHARBOR_RUN_AS_ROOT`, the drop-in it writes, and what it changes.

### Changed
- `plugins/scanners/README.md` — **On a native install**: the plugin downloads a pinned,
  checksum-verified binary into its own folder and removes it with the plugin; the manual
  install commands are gone.
- `plugins/scanners/trivy/*`, `plugins/scanners/grype/*` — the **Binary missing** row and the
  README describe the plugin's own download.
- `getting-started/installation/native-linux.md` — Troubleshooting item 3 notes that a root
  install needs no drop-in.

## 2026-10-05 07:41 — McHarbor 2026.10.01 — Native installs: time zone and self-update

### Changed
- `settings/general/general/README.md` — on a native install the time zone follows the host;
  the picker is disabled and a save is refused with reason `native_timezone`.
- `settings/general/general/reference.md` — the native time zone row.
- `settings/updates/mcharbor/README.md` — a native install does not update itself; **Update
  now** is not offered.
- `getting-started/installation/native-linux.md` — Upgrading says what **Settings → Updates →
  McHarbor** does on a native install.

## 2026-10-05 02:54 — McHarbor 2026.10.01 — Native storage paths, agent release builds, editor mounts

### Added
- `getting-started/installation/native-linux.md` — Troubleshooting item 3: a local storage
  location failing with `read-only file system` needs a `ReadWritePaths=` systemd drop-in and
  write access for the `mcharbor` user, with three checks.
- `agents/releases/README.md` — **Builds**: a newer build of a stored version is pulled and
  replaces it while the stored one keeps being served.
- `screens/networks/fields.md` — **IPAM values**, with the refusal each field gives.
- `settings/integrations/catalogs/README.md` — **Adding a catalog** (a duplicate URL is refused)
  and **Plugin icons from a remote catalog**.
- `screens/containers/actions.md` — **Mounts** and **Exposed ports** in the configuration editor.
- `plugins/scanners/README.md` — **On a native install** (installing `trivy` and `grype`) and
  **A fresh install's scanner list**.

### Changed
- `settings/integrations/storage/reference.md` — the native read-only row.
- `agents/releases/README.md`, `settings/updates/agents/reference.md` — a failed check retries
  after 15 minutes and at start-up; notifications are per version and build; the Download action
  names the build; the default source is the mcharbor.eu mirror.
- `settings/updates/agents/README.md` — **Update agents now** needs `environments.manage` and
  reports the queued count; per-agent statuses are listed.
- `screens/networks/fields.md`, `screens/networks/troubleshooting.md` — a native McHarbor lists
  its own host's interfaces; an agent that does not answer is named.
- `settings/integrations/catalogs/README.md` — the built-in catalogs are the two mcharbor.eu
  rows, and the app catalog moved to `/catalog/apps/index.json`.
- `screens/store/troubleshooting.md`, `settings/integrations/catalogs/reference.md` — duplicate
  catalog and remote icon rows.
- `screens/containers/self-protection/README.md` — what counts as McHarbor's own on a native
  install.
- `plugins/scanners/trivy/*`, `plugins/scanners/grype/*` — install-neutral wording, a
  **Binary missing** row, and the native log command.

### Removed
- `settings/updates/agents/reference.md` — the claim that an agent without a data volume claims
  a fresh licence seat; agents claim none.

## 2026-10-04 22:41 — McHarbor 2026.10.01 — Agent builds land in `agents/`

### Changed
- `agents/installation/README.md`, `agents/releases/README.md`,
  `agents/releases/mirror-contract.md`, `cli/README.md`, `development/workflow/README.md` — the
  repository folder that holds built agent binaries and the agent catalog is `agents/` (was
  `agent/`), including `agents/catalog/`.

## 2026-10-04 16:44 — McHarbor 2026.10.01 — Native installs pin the data encryption key

### Changed
- `getting-started/installation/native-linux.md` — the filesystem table lists
  `/etc/mcharbor/encryption-key.env`, and Configuration explains that the installer pins
  `ENCRYPTION_KEY` there (the existing key on an upgrade, a new one on a fresh install).

## 2026-10-04 16:03 — McHarbor 2026.10.01 — A licence seat is an environment

### Changed
- `settings/system/license/README.md` — **Seats** now says a seat is an active environment,
  agent or direct; agents claim no seat and ignore `MCHARBOR_LICENSE_KEY`. The instance-id and
  machine-fingerprint deduplication text is gone with the agent check-in.
- `settings/system/license/reference.md` — the "seats higher than expected" row.
- `agents/prerequisites/README.md`, `agents/operations/README.md` — the licence-seat sections.
- `agents/configuration/README.md`, `getting-started/configuration/README.md` —
  `MCHARBOR_LICENSE_KEY` is documented as ignored on an agent.
- `agents/troubleshooting/README.md`, `how-to/agent/README.md` — the seat-related rows.

### Removed
- `MCHARBOR_LICENSE_SERVER` from the agent variable tables.

## 2026-10-04 10:49 — McHarbor 2026.10.01 — Setup token on a native install

### Changed
- `getting-started/first-run/README.md` — the setup token is read with `docker logs mcharbor`
  (container) or `journalctl -u mcharbor` (native), and the native installers print it when
  they finish; no token printed means an account already exists.
- `getting-started/first-run/troubleshooting.md` — the stale-token fix and the recommended
  order name the native command too.
- `getting-started/installation/native-linux.md` — section 2 shows the `Setup token: …` line
  the installers print and how to read it again after a restart; the "1.1 GB, 61 bundled
  plugins" size claim is replaced, since native artifacts carry no plugins.
- `getting-started/README.md`, `api/conventions/authentication.md` — the token's location
  names the native journal alongside the container log.

## 2026-10-02 12:47 — McHarbor 2026.10.01 — Release changelog

### Changed
- The root `CHANGELOG.md` section `[2026.10.01]` is now the release summary against 2.0.8 —
  Added, Changed (Breaking entries first), Fixed and Security — replacing the per-change record
  that accumulated under that heading.
- `getting-started/changelog/README.md` — the current release is 2026.10.01 (was 3.0.0), with
  its breaking-changes table rebuilt from the root entry and the 3.0.0 history row replaced;
  the Versioning section describes calendar versions, `BUILD`, and the agent sharing the
  app's version; the intro no longer calls the root file function-level detail.
- Every entry stamped `McHarbor 2026.09.01` now reads `McHarbor 2026.10.01`: 2026.09.01 was
  never released, and that work ships as 2026.10.01. Entry bodies are unchanged.

## 2026-10-02 02:34 — McHarbor 2026.10.01 — Credentials at rest, plugin mount guard, i18n authoring

### Changed
- `settings/integrations/snapshots/reference.md` — secret driver fields are encrypted, shown as "Stored - leave empty to keep", and a save for a driver whose plugin is not loaded is refused with `err.plugin.not_installed`.
- `api/endpoints/backups.md` — snapshot-config secrets: `secretsSet` in responses, a blank secret keeps the stored value, the not-installed refusal, and the boot-time encryption of existing rows.
- `backup/screen/troubleshooting.md` — a row for the not-installed refusal.
- `plugins/snapshots/synology/configuration.md`, `plugins/snapshots/truenas/configuration.md` — the secret-field paragraph names `secretsSet` and the edit-form label.
- `api/settings/README.md`, `api/settings/reference.md`, `api/endpoints/settings.md` — `tls_cert`, `tls_key` and `webpush_vapid_private` are refused by the generic write routes (`400 err.protected_target`), with the TLS and push routes named, and the audit diff stores `[redacted]`.
- `api/endpoints/gitops.md` — desired-state `config` and `spec` are encrypted at rest, credential-looking values are masked in responses, and sending the mask back keeps the stored value.
- `getting-started/upgrading/README.md` — a section on the first-start encryption of legacy plaintext credentials: idempotent, skipped without an encryption key, writes refused instead of stored in plaintext.
- `getting-started/configuration/README.md` — the `ENCRYPTION_KEY` row links to that section.
- `api/endpoints/identity.md` — a configuration change is refused with `err.identity.invalid_type` while the provider's plugin is not loaded.
- `development/contracts/search/README.md` — the palette's "search source is unavailable" line, and the plugin-view failure message with Retry, in the failure table.
- `settings/system/about/README.md`, `screens/containers/fields.md` — the About tab and the exec history panel show the server's error with Retry.
- `how-to/plugin/frontend.md`, `development/plugins/developing.md`, `development/plugins/authoring-cli/README.md` — every plugin root is created through `guardedRoot(el)` from `mountGuard.tsx`, and `plugin dev new` scaffolds it.
- `how-to/plugin/isolation-and-families.md` — D-1 notes that each frontend carries its own `mountGuard.tsx`.
- `development/frontend/components.md`, `development/frontend/data-and-state.md`, `development/frontend/README.md` — `PluginMountBoundary`, per-module `queryKeys.ts` factories, and the i18n rule that new keys are translated when authored (no `sync-locales.mjs --write`; `i18n/untranslated` and `i18n/key-unused` findings).

## 2026-10-01 23:08 — McHarbor 2026.10.01 — Search sources fan out per object

### Changed
- `development/contracts/search/README.md` — `fan_out` (one request per item of another list, with the item id as a query parameter and a field on every row), `{field}` placeholders in a link's query, two new failure rows, and the tests that cover it.
- `development/contracts/capabilities.md` — the validation summary names `fan_out` and its exclusivity with `env_scoped`.

## 2026-10-01 22:55 — McHarbor 2026.10.01 — CLI: host OS updates

### Added
- `cli/README.md` — the Commands table gains a Host updates row for `host updates check`, `status`, `apply` and `schedule`, with `--env` or `--self` and the permission keys each route needs.

## 2026-10-01 20:20 — McHarbor 2026.10.01 — KVM VNC console for native clients

### Changed
- `cli/README.md` — the `kvm` row names `vm console <id> --console vnc [--listen …]`, which serves a VM's VNC console on a local port for a native VNC client.

## 2026-10-01 16:40 — McHarbor 2026.10.01 — Reporting collects with API keys

### Changed
- `how-to/reporting/README.md` — the loopback collector forwards the session cookie, or the Authorization header for an API-key request.

## 2026-10-01 15:42 — McHarbor 2026.10.01 — Host metrics history source

### Changed
- `api/endpoints/host.md` — `GET /api/metrics/host/history` now says CPU and memory are the host's own reading, with container sums only as the fallback, and I/O stays a container sum.

## 2026-10-01 15:06 — McHarbor 2026.10.01 — Host updates: remove unused packages

### Added
- `screens/environments/README.md` — a *Removing unused packages* section: the option runs the manager's cleanup only after a successful upgrade, the per-manager command table, and that apk and zypper ignore it.

### Changed
- `screens/environments/README.md` — the apply confirmation now carries three options, not two.
- `settings/updates/host-os/README.md` — the **Update host** confirmation and the saved defaults name **Remove unused packages**.
- `api/endpoints/host.md` — the `os-updates/apply` and `os-updates/schedule` bodies list `removeUnused`.

## 2026-10-01 15:05 — McHarbor 2026.10.01 — Estate sources on environment cards

Written against `src/backend/core/plugincaps/{capabilities,estate}.go`, `src/frontend/modules/environments/{hooks/useEnvironmentEstate.ts,components/EnvironmentEstateRow.tsx}` and the `estate-source` declarations in `addon-kvm`, `addon-lxc` and `addon-swarm`.

### Added
- `development/contracts/capabilities.md` — a new *Estate sources* section: the optional card fields (`label_key` + `i18n_ns`, `icon`, `env_scoped`, `running_field` + `running_values`, `link`), how the host counts per environment, and that a failing or empty source is left off the card.

### Changed
- `development/contracts/capabilities.md` — the `estate-source` row of the kinds table lists the card fields, and `environment_field` is no longer required when `env_scoped` is set.

## 2026-10-01 14:00 — McHarbor 2026.10.01 — Display profiles, hot-plug, console resize, palette search

Written against `src/plugins/Plugins/addon-kvm/{plugin.yaml,backend/{display_profiles,hotplug,service,cli}.go,frontend/src/{console-resize.ts,components/Console*.tsx}}`, `src/frontend/resources/{utils/keyboardCapture.ts,hooks/useShortcuts.ts,layout/useHeaderOverlays.ts}`, the `search-source` and `search-action` declarations of addon-kvm, addon-lxc, addon-swarm, addon-kubernetes and addon-compute, `scripts/development/catalog.sh`, and `modules/appstore/plugins/upgrade.go`.

### Added
- `virtual-machines/display-profiles/README.md` — the Display profiles tab, the three built-in presets and their values, the custom profile options and the values QEMU accepts, delete with `?reassign=true`, the per-machine choice, routes with `vm.view` / `vm.manage`, audit actions, the CLI group and what goes wrong.
- `virtual-machines/hot-plug/README.md` — what can change on a running machine and with which route and key, the `hotplug` host feature, and every refusal with its `err.vm.hotplug_*` code.

### Changed
- `virtual-machines/README.md` — the Hardware paragraph no longer says hot-plug is offered for nothing, and two index rows.
- `virtual-machines/spice-console/README.md` — a *Display size and keyboard* section (resize requirements for SPICE and VNC, keyboard capture) and a link to display profiles.
- `screens/README.md` — the global search entry names the groups and actions plugins add.
- `development/workflow/README.md` — `catalog.sh build --no-cache` in the option table.
- `api/endpoints/store.md` — the update stream row describes the staged preflight and the rollback on a failed install.

## 2026-10-01 13:55 — McHarbor 2026.10.01 — CLI agent host checks and widgets

Written against `src/backend/modules/agent/routes.go`, `modules/widgets/routes.go` and `src/cli/internal/cli/cmd_agent_probe.go`, `cmd_widgets.go`.

### Added
- `cli/README.md` — Commands table rows for `agent probe`, `agent install-prereqs` and `widgets catalog/install/remove`, with their permission keys.

## 2026-10-01 13:44 — McHarbor 2026.10.01 — Frontend development section

Written against `src/frontend/resources/{layout,components,hooks,stores,theme,utils}/`, `core/router/{lazyPages,appRoutes,routeHelpers}.tsx`, `core/api/{client,env-url}.ts`, `core/auth/{usePermission,permissionKeys,RequirePermission}`, `core/i18n/i18n.ts`, `main.tsx`, `app.css`, `eslint.config.js`, `test/`, `modules/volumes` and `modules/containers/pages/ContainersPage.tsx`, and the addon-kvm `frontend/vite-plugin.config.ts` for the plugin notes.

### Added
- `development/frontend/README.md` — the section index, where shared code lives, a ten-step checklist for a new page, and what a plugin bundle shares with the host.
- `development/frontend/layout.md` — the app shell, route registration and handles, the scroll contract, page-header shapes, `TablePage`, the Volumes example, section-level layout pieces, navigation entries, plugin pages and path aliases.
- `development/frontend/components.md` — a "pick the right one" table and every primitive in `@resources/components/ui` by category, with the lint rules that enforce them.
- `development/frontend/composites.md` — tables, column meta and batch actions, row failures, editors and schedules, charts, logs, shared dialogs, the terminal, shell-mounted components, domain folders and the lazy `*Impl` pattern.
- `development/frontend/data-and-state.md` — the API client and envelope, `assertSuccess` and `refusalOf`, query and mutation patterns, the global toast, the shared hooks by area, Zustand stores, permissions, i18n, utilities and test helpers.
- `development/frontend/styling.md` — semantic tokens, status and chart colours, control heights, no inline styles and `cssVars`, class composition, themes and motion.

### Changed
- `development/README.md` — a Frontend row in the pages table.
- `development/workflow/README.md` — the toolchain row now says Node 26, matching `docker/Dockerfile`; the `resources/` row links the new section.
- `development/hooks/README.md` — the shared-hooks count is about 106, with a link to the hooks listed by area.

## 2026-10-01 13:40 — McHarbor 2026.10.01 — Symlinked bind sources on the self environment

Written against `core/docker/selfprotect_symlink.go`, `selfprotect.go` (`hostAccessOn`), `selfprotect_bind.go` (`VolumeHostAccessOn`), `selfprotect_compose.go` (`ComposeHostAccessReasonsProbe`), and the stacks and workflows compose checks.

### Changed
- `screens/containers/self-protection/README.md` — the residual symlink gap is replaced by a *Symlinks* section: the `host_bind_symlink` reason, the daemon-side helper (image choice, flags, mounts, the per-component walk), when it runs, its fail-closed cases, the native and demo behaviour, and the remaining check-to-create window with its mitigation; two new rows under *What goes wrong*.

## 2026-10-01 07:40 — McHarbor 2026.10.01 — Defect tracker pointer

### Changed
- `screens/containers/self-protection/README.md` — the residual symlink gap now points at TOFIX-03 in `.references/tofix/2026-10-01-0737-open-defects.md`.

## 2026-10-01 07:15 — McHarbor 2026.10.01 — Search palette contract for plugins

Written against `core/plugincaps/{search,capabilities}.go`, `src/cli/internal/cli/plugin_validate_search.go`, `src/contracts/plugincaps/{cases,search_icons}.yaml`, the frontend `resources/components/{useGlobalSearchPluginData,globalSearchPlugins}.ts`, `GlobalSearchPluginGroups.tsx`, `GlobalSearch.tsx`, and the `search-source` / `search-action` declarations and `?action=` handling in addon-kvm, addon-lxc, addon-compute, addon-kubernetes and addon-swarm.

### Added
- `development/contracts/search/README.md` — the `search-source` and `search-action` kinds: fields, validation, how the palette fetches, gates and links, how a plugin page receives `?action=`, the shipped declarations, and what goes wrong.

### Changed
- `development/contracts/capabilities.md` — two rows in the kinds table, fourteen kinds in *Validation* plus the search rules, and the new frontend test under *Tests*.
- `development/contracts/README.md` — the two kinds in the capability row, and a *Search palette* index entry.

## 2026-10-01 01:52 — McHarbor 2026.10.01 — Workflow self-protection, plugin lifecycle subjects, parallel catalog builds

Written against `modules/workflows/{selfguard,handler,service,executor,storage_backup_nodes}.go`, `core/i18n/codes_hostself.go`, `core/selfguard/selfguard.go`, migrations 129 and 130, `modules/lifecycle/service.go`, the `lifecycle-subject` declarations in the addon-kvm and addon-compute `plugin.yaml`, the frontend `WorkflowSelfHostNotice.tsx`, `useLifecycleSubjectTypes.ts` and en locale bundles the labels are quoted from, and `scripts/development/catalog.sh`. The root changelog and readme were updated in the same pass.

### Added
- `workflows/self-protection/README.md` — the guarded nodes, the save-time check on update and import, the `selfHostAuthorized` mark and badge, the run-time refusal and `target=unknown`, and what goes wrong.

### Changed
- `workflows/README.md`, `workflows/troubleshooting/README.md` — index row, and the two workflow refusal codes.
- `screens/containers/self-protection/README.md` — a Workflows row in the paths table, workflows as the exception under *Nobody behind it*, and a troubleshooting row.
- `screens/lifecycle/README.md`, `screens/lifecycle/fields.md`, `screens/lifecycle/troubleshooting.md` — plugin subject types come from `lifecycle-subject` declarations, the filter labels as the interface shows them, and a missing-plugin row.
- `development/workflow/README.md` — `catalog.sh build` options: `-j`/`--jobs`, `--timeout`, `--keep-going`, `--index-anyway`, `--no-cache`, and where the per-plugin logs go.

## 2026-09-30 22:35 — McHarbor 2026.10.01 — Agent RPC contract replaces the compute handle

Written against `src/backend/core/agent/pluginroutes.go`, `core/agentrpc/agentrpc.go`, `core/plugincaps/{agentrpc,console,reboot_drain}.go`, `src/contracts/plugincaps/agent_rpc_ceiling.yaml`, `src/contracts/agentwire/`, and the `agent-rpc` and `reboot-drain` declarations in the addon-kvm and addon-lxc `plugin.yaml`.

### Added
- `development/contracts/agent-rpc/README.md` — the `agent.rpc` token and `agent-rpc` capability, the local interface a plugin asserts `HostContext.AgentRPC` against, `Available`, `Call` and `Subscribe`, the five error codes, the ceiling, budgets and floors, the goldens and `wire_agent.go`, and what goes wrong.
- `development/contracts/reboot-drain/README.md` — the `reboot-drain` capability, the two routes and their answers, the shipped declarations and keys, and the reboot inventory's `guests` count.

### Changed
- `development/contracts/capabilities.md` — the `console-subject`, `agent-rpc` and `reboot-drain` kinds, a Console subjects section, and the validation rules for all three.
- `development/contracts/host-services.md`, `development/contracts/README.md`, `development/hooks/README.md`, `development/plugins/developing.md` — `AgentRPC` replaces `ComputeRPC`, and a new agent message is a reviewed addition to the ceiling rather than a `GuestCommand` field.
- `development/contracts/adapters.md` — the compute types section now points at the agent RPC page.
- `how-to/plugin/manifest.md`, `plugins/addons.md`, `compute-foundation/{README,configuration}.md`, `virtual-machines/configuration.md`, `system-containers/configuration.md`, `development/add-ons/{compute-foundation,virtual-machines,system-containers}.md` — the token is `agent.rpc`, not `compute.rpc`.
- `api/endpoints/host.md` — the reboot inventory reports one `guests` count.
- `cli/README.md` — `kvm reboot-drain` and `lxc reboot-drain`.

### Removed
- `development/contracts/adapters.md` — the `specs/compute` type list, and the `ComputeRPC` method table in `host-services.md`, with `PoolFileHandle` and `ConsoleStream`.

## 2026-09-30 19:50 — McHarbor 2026.10.01 — Cross-platform report facets, VM console and migration, self-protection, downloaded widgets

Written against `core/plugin/specs/reporting/facets.go`, `core/pluginruntime/reporting_facets.go`, the Reporting plugin's `backend/{facets,facets_docker,build_facets,adhoc_facets}.go` and `migrations/0007_facet_report_keys.sql`, each platform plugin's `reports_facets.go`, addon-kvm's `backend/{migrate,migrate_handler,spice,service}.go` and `plugin.yaml`, the agent's `compute_spice_linux.go`, `compute_vm_clone_linux.go`, `compute_vm_migrate_linux.go`, `compute_ct_snapshot_linux.go` and `compute_capability_linux.go`, `core/plugincaps/runbook.go`, `core/docker/selfprotect*.go`, `core/selfguard`, the reconciler, backups, stacks, networks, volumes and runbooks self-guard files, `core/rbac/{abac_decision,abac_attributes}.go`, `core/widgetsig`, `modules/widgets`, `cmd/pluginsigner`, `modules/agent/{prereq,escalate}.go`, `modules/appstore/catalog.go`, the comm plugins' backends and i18n bundles, and the frontend components and en locale bundles the labels are quoted from. The what's-new overlay and the root changelog and readme were updated in the same pass.

### Added
- `reporting/facets/README.md` — the six facets, who answers which, which reports print them, what a missing or failing platform looks like, and the twelve removed report types with their replacements and the migration's effect.
- `how-to/reporting/facets/README.md` — the plugin contract: `FacetDecl`, `ReportFacetContributor`, the row types, `ReportFacetHandle`, permissions, registration validation, audit, and the ABI cost.
- `virtual-machines/spice-console/README.md` — the SPICE console, the `.vv` file, the QEMU requirement, per-channel tickets, serial resize.
- `virtual-machines/clone/README.md` — full and linked copies, what the copy gets, the linked-copy rules.
- `virtual-machines/migration/README.md` — cold and live moves, the refusals, `vm.migrate` on both hosts, what must be reachable, rollback.
- `system-containers/snapshots-and-clones/README.md` — rootfs-copy snapshots, revert, and why a clone is a full copy.
- `how-to/widgets/downloaded/README.md` — catalog layout, the signed manifest, `pluginsigner -widget`, `dashboard_widgets.install`, re-verification.
- `screens/containers/self-protection/README.md` — every operation that needs `host_self.manage` on McHarbor's environment: create-time rules, the host-path denylist, namespace joins, disguised volumes, compose fields, and each module that can act on a McHarbor container.

### Changed
- `reporting/reports/README.md` — the twelve removed keys are gone, `container-inventory` is `workload-inventory`, a section on the cross-platform sections, and a note on the removals.
- `reporting/README.md`, `how-to/reporting/README.md`, `development/add-ons/reporting.md` — links and index rows for the facets, the migration count, the fourth way to add a report, and the source layout.
- `virtual-machines/README.md`, `virtual-machines/qemu-features.md`, `compute-foundation/README.md`, `compute-foundation/host-requirements.md` — the console kinds, `vm.migrate`, the five new capability keys, the optional SPICE-capable QEMU, and report counts.
- `system-containers/README.md`, `docker-swarm/README.md`, `kubernetes/README.md` — index rows and the pick lists on the workflow nodes.
- `oncall/runbooks/plugin-steps.md`, `development/contracts/capabilities.md` — attach kinds declared by plugins, *Provider not installed*, and the shipped picker sources.
- `plugins/communication/README.md`, `plugins/communication/signal/README.md`, `plugins/communication/whatsapp/README.md`, `plugins/communication/zammad/README.md`, `plugins/communication/zammad/configuration.md` — attachments and the Zammad reopen window.
- `development/contracts/types.md`, `development/plugins/developing.md` — `LabelKey` and `HelpKey`, translated setup guides.
- `api/endpoints/containers.md`, `screens/containers/actions.md`, `screens/containers/README.md`, `screens/enterprise/compliance.md` — the wider self-protection list, peer and protected-container detection, and direct grants in the compliance section.
- `screens/security/policies/README.md`, `api/endpoints/governance.md` — `env.*` at the route gate, the fail-closed rule, and the explanation sharing the evaluator.
- `api/endpoints/monitoring.md`, `api/conventions/permissions.md`, `screens/store/README.md`, `how-to/widgets/README.md` — the widget catalog routes and `dashboard_widgets.install`.
- `screens/store/fields.md` — the identity placeholders in environment values.
- `agents/prerequisites/README.md`, `agents/installation/README.md` — the SSH prerequisites panel and how privileged commands run.
- `cli/README.md` — the `kvm` row (`vm clone --linked`, `vm migrate`, `vm migration`, `vm migration-cancel`, `vm.migrate`) and the `lxc` row (`container clone --linked`).

### Removed
- `reporting/reports/README.md` — the rows for `kvm-inventory`, `kvm-protection`, `kvm-allocation`, `lxc-inventory`, `lxc-utilisation`, `lxc-protection`, `compute-inventory`, `compute-capacity`, `kubernetes-cluster-health`, `kubernetes-workload-inventory`, `swarm-cluster-health` and `swarm-service-inventory`, and the System container protection description.

## 2026-09-30 17:06 — McHarbor 2026.10.01 — Image export needs `images.manage`, plugin `audit_severity`

Written against `modules/images/routes.go`, the frontend `useImagesPageConfig.tsx` and `ImageDetailHeader.tsx`, `core/plugin/contract/manifest.go` (`RouteDecl.AuditSeverity`), `core/plugin/host/manifest_schema.go` (`KnownAuditSeverity`) and `manifest-v1.schema.json`, `core/pluginruntime/loader.go`, `core/audit/middleware.go` (`PluginFallbackMiddlewareWithSeverity`), the CLI's `plugin_validate.go`, `src/contracts/pluginroutes/audit_severity.yaml`, and the addon-swarm and addon-kvm manifests.

### Changed
- `screens/images/actions.md` — **Export** needs `images.manage` and renders disabled without it.
- `screens/images/fields.md` — Export is unmasked, which is why it takes `images.manage`.
- `api/endpoints/images.md` — export moved from the `images.view` row to `images.manage`.
- `how-to/plugin/manifest.md` — `audit_severity` row: the four values, the default, failed calls, and that reveal routes declare `warning`.
- `development/contracts/types.md` — `RouteDecl.AuditSeverity` and `host.KnownAuditSeverity`.
- `virtual-machines/README.md` — the cloud-init reveal is audited at warning severity.

## 2026-09-30 17:00 — McHarbor 2026.10.01 — `host_self.manage` guards McHarbor's own containers

Written against `core/docker/{selfprotect,selfprotect_compose,protection}.go` (`ClassifyContainer`, `ClientPool.SelfHost`, `HostConfigReasons`, `ComposeHostAccessReasons`), `core/selfguard/selfguard.go`, `core/i18n/codes_hostself.go`, `core/rbac/permissions_host_self.go`, `modules/containers/{selfprotect,handler,service,move_service}.go`, `modules/terminal/handler.go`, `modules/debugshell/{selfguard,handler,service}.go`, `modules/stacks/{selfguard,handler,service}.go`, `modules/compliance/{evaluate,service,routes}.go`, `docker/Dockerfile`, and the frontend `useHostSelfGate`, `ContainerActionsCell`, `ContainerDetailActions` and `PermissionEditor.parts`.

### Changed
- `api/endpoints/containers.md` — new *McHarbor's own containers* section: which operations need `host_self.manage` on which targets, the create-time rule on the environment McHarbor runs on, the stack and compose rules, the `403` codes and body, the `host_self.denied` audit entry, the detection rules, and that the existing lock still stands for a holder; the *Protected containers* note says the unlock flag is not a permission.
- `screens/containers/actions.md` — new *McHarbor's own containers* section: the gated buttons, the tooltip, host-access containers, and the create-time refusal; the Rename row names the key.
- `api/conventions/permissions.md` — the `host_self.manage` row now covers McHarbor's own containers.
- `screens/stacks/README.md` — permissions table: `host_self.manage` for a stack that runs McHarbor and for a compose file that reaches McHarbor's host.
- `screens/enterprise/compliance.md` — new `selfEnvAccess` section row and two *What goes wrong* rows.

## 2026-09-30 16:57 — McHarbor 2026.10.01 — Redaction review fixes

Written against `modules/containers/{protected_paths,recreate_guard,exec_history,handler,secretpath}.go`, `modules/stacks/{routes,handler,service,compose_redact}.go`, `core/envredact/{envredact,compose,registry}.go`, `core/procenv/procenv.go`, `secret_env.go`, `core/auth/{auth,sudo}.go` and migration `128_hash_session_ids.sql`, `modules/reconciler/{plan,proposals,responder,apply,handler_propose}.go`, addon-swarm's `backend/{service_env,service_services,handler_services}.go`, and the Terraform provider's `resource_stack.go`.

### Changed
- `screens/containers/fields.md` — the widened classifier (names, value shapes, flags and small numbers not masked by name); the recreate refusal when an unrevealable value is kept while the image or command changes; new *Files tab: protected paths* section (key files refused everywhere; data directory, `/run/secrets`, `_FILE` directories, data-volume mounts and `/proc` need `system.manage`; audit actions); new *Exec history* section.
- `screens/stacks/README.md` — stack routes decide on the stack's own environment; create refuses another `environmentId`; the adopt preview masks, `?reveal=true` and adopting a masked preview need `containers.manage`; the Compose tab's wider masking, McHarbor detection and the moved-value and alias refusals; the Environment tab uses the shared classifier.
- `docker-swarm/README.md` — the reveal's own warning-severity audit row, stack-deployed McHarbor detection, pinned values, and the update refusal.
- `getting-started/configuration/README.md` — all four root-of-trust variables leave the process environment; whitespace-only `MCHARBOR_SECRET` is unset; the compose child allow-list.
- `screens/reconciler/plans-and-enforcement.md` — what a stored recreate step shows and that approval rebuilds the environment.
- `terraform/resources.md` — `mcharbor_stack.compose` is sensitive, and what a refresh without `stacks.manage` does.
- `api/conventions/authentication.md` — the session id is stored as its SHA-256; the listed id is that hash.

## 2026-09-30 16:15 — McHarbor 2026.10.01 — Image env and VM user-data redaction, Kubernetes checked

Written against `modules/images/{envreveal,handler,routes}.go`, `modules/workflows/service.go` (`imageEnvForRun`), the CLI's `image get --reveal`, the frontend `ImageEnvList` and `ImageHistoryCard`, addon-kvm's `backend/{cloudinit_redact,handler,scan,cli}.go` and `plugin.yaml`, and addon-kubernetes' `backend/{service,model}.go` (read to confirm no env or Secret data is returned).

### Changed
- `screens/images/fields.md` — new *Environment values* section: masked `Config.Env` and history values, **Reveal values** under `images.manage` with the `reveal_env` audit entry, and that Export is not masked.
- `screens/images/actions.md` — **Reveal values** row; **Export** corrected to `images.view`, the key the route actually takes, and notes the archive carries the configuration.
- `api/endpoints/images.md` — inspect and history mask credentials; `?reveal=` needs `images.manage`.
- `kubernetes/README.md` — no view returns container env, Secret data or ConfigMap data, so there is no reveal route.
- `virtual-machines/README.md` — cloud-init user-data is masked on the detail; `GET /api/vms/{id}/cloud-init` and `mcharbor kvm vm cloud-init` under `vm.manage`, audited; a masked create is refused.
- `cli/README.md` — the Images row covers `image get --reveal`; the `kvm` row covers `vm cloud-init`.

## 2026-09-30 15:58 — McHarbor 2026.10.01 — `stack env` CLI, PostgreSQL password from a Docker secret

Written against the CLI's `cmd_stack_env.go`, the stack env routes in `modules/stacks/{routes,handler,envsec}.go`, `docker-compose.ha.yml` and `deploy/compose/external-database/docker-compose.yml`.

### Changed
- `cli/README.md` — new *Stack environment* row: `stack env`, `stack env set`, `stack env unset`, their permission keys, the reveal behaviour, and the risk level of each.
- `settings/system/high-availability/README.md` — *Shared keys as Docker secrets* covers `secrets/postgres_password` (`POSTGRES_PASSWORD_FILE`), states that it and the password inside `secrets/mcharbor_db_dsn` must agree, and gives the existing-cluster step.
- `getting-started/deployment/compose.md` — *External database* now creates `secrets/postgres_password` and `secrets/mcharbor_db_dsn`, explains that PostgreSQL reads its password only at first initialisation, and gives the existing-deployment step; the Active/passive section lists the fourth secret file.
- `getting-started/upgrading/README.md` — the HA section gains the operator step: write the current PostgreSQL password into `secrets/postgres_password` before recreating, and the same change for the external-database file.

## 2026-09-30 15:35 — McHarbor 2026.10.01 — VM import conversion progress

Written against `core/agent/{compute,transport}.go` (`ImportProgressSink`, the `vm_import_progress` route), `core/compute/rpc.go` (`HandleImportProgress`, `ImportProgress`, `ImportProgressSource`), addon-kvm's `backend/{import_jobs,imports}.go` (`watchImportProgress`, `advanceImportProgress`) and the agent's `compute_import_linux.go` (`sendPullProgress`, `reportConvertProgress`).

### Changed
- `virtual-machines/README.md` — the import bullet says the percentage beside **Converting** is the host's relayed qemu-img progress, spread across every disk and never moving backwards within a step, and that in an active/active cluster it appears only when the node running the import holds that host's agent.

## 2026-09-30 15:34 — McHarbor 2026.10.01 — Compose and Swarm env redaction, `MCHARBOR_DB_DSN_FILE`

Written against `core/envredact/compose.go` (`Policy.Compose`, `RestoreCompose`), `modules/stacks/{handler,service,compose_redact,routes}.go`, `core/i18n/codes_envredact.go`, `core/config/config.go` (`MCHARBOR_DB_DSN_FILE`), `docker-compose.ha.yml`, `ha.env.example`, and addon-swarm's `backend/{service_env,service_services,handler_services,cli}.go`, `plugin.yaml` and `frontend/src/components/ServiceEnvList.tsx`.

### Changed
- `screens/stacks/README.md` — the Compose tab masks credential-looking `environment:` values, **Reveal values** under `stacks.manage` with the `reveal_compose` audit entry, and what saving a masked file keeps or refuses.
- `docker-swarm/README.md` — service env is masked; **Reveal values** / `mcharbor swarm service env` under `swarm_services.manage`, audited as `plugin.swarm.service.env_revealed`; route count 46.
- `getting-started/configuration/README.md` — `MCHARBOR_DB_DSN_FILE` row; the `MCHARBOR_DB_DSN` row notes it is visible in `Config.Env`.
- `settings/system/high-availability/README.md` — *Shared keys as Docker secrets* covers the DSN file, its password, and moving an existing cluster.
- `getting-started/deployment/compose.md` — the HA template needs `secrets/mcharbor_db_dsn` as well.
- `getting-started/upgrading/README.md` — new *HA clusters on the Compose template* section with the operator action before recreating.
- `cli/README.md` — the Stacks row covers `stack compose --reveal` and its key.

## 2026-09-30 14:24 — McHarbor 2026.10.01 — Container and stack environment redaction, `_FILE` secrets

Written against `core/envredact/envredact.go`, `modules/containers/{envreveal,handler,routes,service}.go`, `modules/stacks/{handler,envsec,routes}.go`, `core/config/config.go` (`resolveSecretFiles`), `secret_env.go`, `core/docker/{protection,self_update_scheduler}.go`, `modules/reconciler/{drift,source_pr}.go`, `docker-compose.ha.yml`, `ha.env.example`, the CLI's `container get --reveal`, and the frontend `ContainerEnvList` and stack `EnvironmentVarRow`.

### Changed
- `screens/containers/fields.md` — new *Environment values* section: which values are withheld, **Reveal values** under `containers.manage` with the `reveal_env` audit entry, the values no reveal returns on a McHarbor container, the Files-tab refusal of McHarbor's key files, and that editing keeps untouched values.
- `screens/containers/actions.md` — **Reveal values** row; **Edit** notes that masked values are kept.
- `screens/stacks/README.md` — revealing masked environment values needs `stacks.manage` and is audited; values carrying McHarbor's own keys stay masked.
- `getting-started/configuration/README.md` — `MCHARBOR_SECRET_FILE` and `ENCRYPTION_KEY_FILE` rows and their refusals; the `MCHARBOR_SECRET` row no longer implies the secret leaves the container — a variable-pinned value stays in `Config.Env` and `/proc/1/environ`; a note on preferring the `_FILE` forms.
- `settings/system/high-availability/README.md` — new *Shared keys as Docker secrets* section: the template's `_FILE` variables, creating the files, and moving an existing cluster without changing its keys.
- `getting-started/deployment/compose.md` — the HA template needs the two secret files before the first `up`.
- `cli/README.md` — the Containers row covers `container get --reveal` and its key.

## 2026-09-30 14:11 — McHarbor 2026.10.01 — Node-to-node hop hardening, third pass

Written against `core/nodelink/{sign,transport,plaintext,nodelink,admission,receive,stats,stream,trigger_fire}.go`, `modules/nodelink/routes.go` and `modules/cluster/readiness.go` (`nodeLinkTransportGate`).

### Changed
- `settings/system/high-availability/active-active.md` — `MCHARBOR_NODE_LINK_CA_FILE` in the requirements note and the settings table, recommended over `MCHARBOR_NODE_LINK_SKIP_VERIFY`, whose row now states what it gives away; `MCHARBOR_NODE_LINK_ALLOW_HTTP` covers accepting plaintext calls; forward steps 3 and 4 (hop label, TLS and live-node checks); "Addressed to one node and one hop" (hop binding, trimmed values, unsignable values); new "Only live nodes"; empty-digest rule for body-less routes; the unauthenticated count described as protecting nothing and the per-node budget as no bound on a stolen secret; receiver-side TLS and the trusted-proxy rule; unknown-length bodies stream and are never signed in part; skip-verify and the http opt-in make streamed bodies and relayed sessions forgeable; `nodeLinkTransport` gate row.
- `getting-started/configuration/README.md` — `MCHARBOR_NODE_LINK_ALLOW_HTTP` row covers accepting plaintext calls; new `MCHARBOR_NODE_LINK_CA_FILE` and `MCHARBOR_NODE_LINK_SKIP_VERIFY` rows.

## 2026-09-30 13:31 — McHarbor 2026.10.01 — Send Email recipients

Written against `modules/workflows/service.go` (`executeSendConfiguredEmail`, `configuredEmailRequest`), `core/notify/dispatcher.go` (recipient override) and `nodes/send-email/index.ts`.

### Changed
- `workflows/nodes/integrations/README.md` — Send Email's **To** is optional with a configured server: empty sends to the channel's saved recipients, filled replaces them for one send; still required with custom SMTP.

## 2026-09-30 13:21 — McHarbor 2026.10.01 — Node-to-node hop hardening, second pass

Written against `core/nodelink/sign.go` (`Authenticate` / `Accept`, canonical v3 with timeout, actor name and inner-header digest, `SignReplyHead`, nonce sweep), `receive.go`, `transport.go`, `stats.go`, `stream.go`, `stream_client.go` (no proxy), `nodelink.go` (TLS required from `New`), `admission.go`, `refusal_log.go`, `modules/nodelink/routes.go` (unauthenticated and per-node budgets), `modules/cluster/readiness.go` (`nodeLinkTransport`), `main.go` and `secret_env.go` (`MCHARBOR_SECRET` unset after config load), `core/pluginslug`, the plugin installer and loader, and the CLI's `plugin dev validate`.

### Changed
- `settings/system/high-availability/active-active.md` — https is required between nodes in every multi-node mode; the signature is checked before the body is read and the nonce spent last; the signed fields now include the actor name, the timeout and every forwarded Docker header; agent and stats replies are signed; streamed bodies rest on TLS; both hops ignore `HTTP(S)_PROXY`; rate limits are charged after verification (per source for failures, per node for verified calls, 1200 for the fire route); the trigger ledger is at-most-once, not exactly-once; the `nodeLinkTransport` gate applies whenever more than one node is live.
- `getting-started/configuration/README.md` — `MCHARBOR_ADVERTISE_URL`, `MCHARBOR_NODE_LINK_ALLOW_HTTP` and `MCHARBOR_NODE_LINK_RATE_LIMIT` rows follow the new rules; `MCHARBOR_SECRET` is removed from the process environment after the configuration loads.
- `how-to/plugin/manifest.md` — the slug `host` is reserved and refused at install, at load and by `mcharbor plugin dev validate`.

## 2026-09-30 05:56 — McHarbor 2026.10.01 — Node-to-node hop hardening

Written against `core/nodelink/sign.go` (audience claim, `Precheck`, signed replies), `nodelink.go` (`CheckRedirect`), `receive.go`, `trigger_fire.go` (leader probe, signed reply, source binding, peer cache), `plaintext.go`, `refusal_log.go`, `modules/nodelink/routes.go` (per-peer rate limits), `modules/workflows/trigger_fire_ledger.go` and migration `127_workflow_trigger_fires.sql`, `trigger_fire_scope.go`, `modules/cluster/readiness.go` (`nodeLinkTransport`), and `main.go`.

### Added
- `settings/system/high-availability/active-active.md` — "How the hops are protected": the addressed node in every signature and the refusal of the older format, header-only refusal before the body is read, per-peer rate limits and the rate-limited refusal log, no redirects, no plain http between nodes, signed replies.

### Changed
- `settings/system/high-availability/active-active.md` — requirements (https advertise address, same build), settings (`MCHARBOR_NODE_LINK_ALLOW_HTTP`, `MCHARBOR_NODE_LINK_RATE_LIMIT`), the signed fields of a forwarded call, the leader probe, plugin key scoping, exactly-once through the shared `workflow_trigger_fires` table (the leader-crash duplicate window is closed), and the `nodeLinkTransport` readiness gate.
- `getting-started/configuration/README.md` — `MCHARBOR_NODE_LINK_ALLOW_HTTP` and `MCHARBOR_NODE_LINK_RATE_LIMIT`; the `MCHARBOR_ADVERTISE_URL` https rule.
- `development/workflows/plugin-nodes/README.md` — `Fire` accepts only the plugin's own keys, checked on the firing node and on the leader.

## 2026-09-30 05:38 — McHarbor 2026.10.01 — Scanner, email and comm aliases leave the workflow host

Written against `core/plugincaps/channel.go`, `picker.go` and `capabilities.go`, `core/notify/dispatcher.go` (`ChannelTypes`), `modules/workflows/channel_caps.go`, `legacy_aliases.go` and `service.go` (`executeSendConfiguredEmail`), `modules/communications/service.go` (`adapterSupportsEmail`), `modules/scans/config_service.go` (`Resolve`), the frontend `ScannerSelect.tsx` / `EmailChannelSelect.tsx` / `nodes/resolve.ts`, and the comm-* manifests.

### Added
- `development/contracts/capabilities.md` — "Channel-type aliases" and "Email channels": the `channel_type` form of `workflow-node-alias`, the `email-channel` kind, their validation against `contributes.forms`, and their consumers.

### Changed
- `development/contracts/capabilities.md` — kinds table (nine kinds), validation list, absent-plugin behaviour, browser and Go helpers, tests.
- `development/contracts/README.md` — `email-channel` in the capability list.
- `development/workflows/plugin-nodes/README.md` — channel-type aliases next to the node alias.
- `workflows/nodes/actions/README.md` — Image Vulnerability Scan's scanner field is `image-scanner-select` over `/api/scans/scanners`; stored engine names.
- `workflows/nodes/integrations/README.md` — Send Email's server list and its no-server behaviour; saved per-provider nodes under Send Message.
- `how-to/nodes/README.md` — `image-scanner-select` in the field-type list.

## 2026-09-30 05:17 — McHarbor 2026.10.01 — Plugin workflow triggers in active/active

Written against `core/nodelink/trigger_fire.go`, `modules/nodelink/routes.go`, `modules/workflows/trigger.go` (`FirePluginTrigger`) and `trigger_forward.go`, and the peer wiring in `main.go`.

### Added
- `settings/system/high-availability/active-active.md` — "Plugin workflow triggers": how a fire on a non-leader reaches the leader, the fire-id de-duplication window and its one residual duplicate case, the retry policy, and what is logged.

### Changed
- `settings/system/high-availability/active-active.md` — the hop table lists `POST /api/internal/node/workflow-trigger/fire`.
- `workflows/triggers/README.md` — "Plugin triggers" states the cluster behaviour and links the section above.
- `development/workflows/plugin-nodes/README.md` — a fire on a non-leader is forwarded rather than starting nothing; `match` runs on the firing node; the payload crosses as JSON with a 1 MiB limit.

## 2026-09-30 04:44 — McHarbor 2026.10.01 — Plugin pick lists and the VM trigger move

Written against `core/plugincaps/picker.go` and `capabilities.go`, `core/pluginruntime/loader.go` (`stampPickerSources`), `modules/workflows/service.go` and `trigger.go` (alias resolution), `src/frontend/modules/workflows/components/PluginSelect.tsx`, and the `addon-kvm` manifest and `backend/vm_trigger.go`.

### Added
- `development/contracts/capabilities.md` — "Picker sources" and "Workflow node aliases": the `picker-source` and `workflow-node-alias` kinds, how the `plugin-select` field lists and gates, and how a stored bare action resolves to a plugin node.
- `virtual-machines/README.md` — "VM trigger": fields, payload, manual runs, and how workflows saved before the move resolve.

### Changed
- `development/contracts/capabilities.md` — the kinds table, validation list, absent-provider behaviour, browser and Go helpers, and test list cover the two new kinds.
- `development/contracts/README.md` — the capability kinds list names `picker-source` and `workflow-node-alias`.
- `development/workflows/plugin-nodes/README.md` — how a plugin trigger fires, the `plugin-select` field, node aliases, the VM trigger in the shipped list, and what an aliased node does with the plugin uninstalled.
- `development/workflows/core-nodes/README.md` — a picker over a plugin's objects is never a core field type.
- `workflows/triggers/README.md` — "Plugin triggers" names the VM Trigger and the plugin it ships with.
- `virtual-machines/README.md` — five workflow nodes, and the manifest's capability declarations.

## 2026-09-30 02:25 — McHarbor 2026.10.01 — Runbook target labels and document-export routes

Written against `core/plugincaps/capabilities.go` and `runbook.go`, `modules/runbooks/targets.go` and the `addon-reporting` manifest.

### Changed
- `development/contracts/capabilities.md` — `document-export` now lists `generate_path` and `export_path`; a runbook `targets[]` entry gains `label_key`, and `i18n_ns` is required with targets as well as steps.
- `oncall/runbooks/plugin-steps.md` — the catalog answers with `targets` (kind and label) instead of `targetKinds`.

## 2026-09-29 22:08 — McHarbor 2026.10.01 — Runbook provider descriptors

Written against `core/plugincaps/runbook.go`, `modules/runbooks/descriptors.go` and the `runbook-provider` blocks in the `addon-kvm`, `addon-lxc`, `addon-compute`, `addon-swarm` and `addon-kubernetes` manifests.

### Added
- `development/contracts/capabilities.md` — "Runbook providers": the `i18n_ns`, `targets` and `steps` fields, what each placeholder and source means, and the rule that a step key is a contract with stored runbooks.

### Changed
- `development/contracts/capabilities.md` — the `runbook-provider` row of the kinds table, the validation list and the test list now cover descriptors.

## 2026-09-29 21:10 — McHarbor 2026.10.01 — Scoped grants, setup token, host keys, self-service gates, removed routes

Written against the 2026-09-29 and 2026-09-30 backend, frontend and CLI changes: `core/rbac`
(`env_scope.go`, `env_path.go`, `permissions_host_self.go`, `RequireSelfService`),
`core/firstrun`, `core/auth/sudo.go`, `src/cli/internal/cli/cmd_app_install.go` and
`cmd_plugins.go`.

### Added
- `cli/reference/mcharbor_app_install.md`, `mcharbor_app_uninstall.md` and `mcharbor_plugin_reconcile.md` — generated reference pages for the three new commands.
- `api/conventions/permissions.md` — "A role granted on one environment carries only environment-scoped keys", "Which environment a route decides on" (`PinEnvToPath`, `PinEnvToResource`, `env_param`), and "Self-service routes" (`RequireSelfService`, `account.self.manage` for API keys).
- `getting-started/first-run/README.md` — "The setup token": where it is printed, the `X-McHarbor-Setup-Token` header, that it changes on every restart, and the SQLite path rule.

### Changed
- `api/conventions/permissions.md` — catalog rows for `host_self.view`, `host_self.manage` and `host_self.reboot`; `account.self.manage` no longer listed among the keys that gate no route (six remain); first-run and self-service rows of the exempt-routes table.
- `api/conventions/authentication.md` — first-run setup token, session id format, step-up failures counting toward lockout, elevation caps and the requester-only read.
- `api/endpoints/auth.md`, `system.md`, `database.md` — setup token on `auth/setup` and `database/setup/test|apply`, the `break_glass` step-up scope and its 5-per-minute throttle, per-user licence notice dismissal.
- `api/endpoints/host.md` — `/api/system/self/...` routes moved to `host_self.*`, and the reboot routes (`os-reboot`) documented for both targets; endpoint count 32 to 39.
- `api/endpoints/users.md`, `governance.md`, `api-keys.md`, `push.md` — elevation caps and read scope, break-glass step-up and `409 already_active`, `account.self.manage` on self-service routes, the 20-device push cap.
- `api/endpoints/store.md`, `monitoring.md`, `README.md` — routes removed, counts corrected (44, 49, 39; 724 in total), the app-store stream body now lists its real fields.
- `api/settings/reference.md`, `observability/README.md`, `development/plugins/architecture.md`, `getting-started/deployment/security-posture/README.md` — the removed routes replaced by their streaming equivalents; a reconcile refusal no longer carries `data.detail`.
- `getting-started/README.md`, `installation/README.md`, `first-run/troubleshooting.md` — the setup token step and its two failure rows.
- `settings/updates/host-os/README.md`, `settings/updates/README.md`, `screens/environments/services/README.md`, `screens/environments/README.md` — McHarbor's own host is gated by `host_self.*`, and the Settings → System → Services tab.
- `screens/security/governance/break-glass.md`, `screens/security/elevations/README.md`, `screens/security/actions.md`, `terraform/resources.md`, `api/endpoints/users.md` — password step-up for break-glass, elevation caps, and scoped roles granting only environment-scoped keys.
- `screens/audit/actions.md`, `settings/general/notifications/README.md`, `settings/monitoring/data-retention/README.md`, `screens/store/actions.md`, `oncall/push.md` — the coverage **Export** button, the **Web push** card, the **Prometheus metrics** card and the plugin **Reconcile** action.
- `settings/system/instance-backup/restore.md`, `backup/screen/troubleshooting.md`, `settings/integrations/storage/reference.md`, `settings/system/license/README.md` — restore refusal reasons, the abandoned-upload failure, per-user licence dismissal.
- `cli/README.md` — `app install`, `app uninstall` and `plugin reconcile` in the Plugins and apps row, with `store_apps.manage` and `plugins.manage`.

### Removed
- `api/endpoints/monitoring.md`, `host.md`, `store.md` — the rows for `GET /api/clock/status`, `GET /api/metrics/containers`, `POST /api/widgets`, `DELETE /api/widgets/{key}`, `POST /api/app-store/install`, `DELETE /api/app-store/installations/{id}` and `POST /api/appstore/plugins/install`, and their request-body entries.

## 2026-09-29 16:20 — McHarbor 2026.10.01 — Plugin permission scope vocabulary

### Changed
- `development/plugins/authoring-cli/README.md` — a plugin's `adds_permissions` scope is `global` (default), `env` or `stack`; the host-only `environment`, `owner` and `account` are refused, and the stale claim that a scopeless key is invisible to the role editor is removed.
- `how-to/plugin/manifest.md` — the `scope` row states the default and how `env` maps to the host's `environment`.
- `development/contracts/types.md` — the `Scope` field comment names the catalog's word for `env`.

## 2026-09-29 15:23 — McHarbor 2026.10.01 — Node 26 toolchain

### Changed
- `how-to/plugin/abi-and-libc.md` — the `frontend` build stage base image is now `node:26-bookworm-slim`.

## YYYY-MM-DD HH:MM — McHarbor <version> — <topic>

### Changed
- `section/page.md` — what is now different.
```

## 2026-09-29 14:55 — McHarbor 2026.10.01 — CLI: validate checks capability declarations

Written against `src/cli/internal/cli/plugin_validate_caps.go`, `src/backend/core/plugincaps/`
and `src/contracts/plugincaps/cases.yaml`.

### Changed
- `development/plugins/authoring-cli/README.md` — the `validate` row and a new "Capability
  declarations" section cover the `contributes.capabilities` check.
- `development/contracts/capabilities.md` — describes how the CLI copy of the rules is held in step
  by the shared fixture file, and the extra step when adding a kind.
- `cli/reference/mcharbor_plugin_dev_validate.md` — synopsis names the capability check.

## 2026-09-29 13:25 — McHarbor 2026.10.01 — Contracts: capability declarations

Written against `src/backend/core/plugincaps/`, `src/backend/core/pluginruntime/loader.go`
(`recordCapabilities`, `UnloadOne`), `src/backend/modules/plugin_runtime/capabilities.go`,
`src/backend/modules/runbooks/`, `src/frontend/core/plugins/capabilities.ts` and the manifests
of `addon-kvm`, `addon-lxc`, `addon-compute`, `addon-swarm`, `addon-kubernetes`,
`addon-reporting` and `identity-ldap`.

### Added
- `development/contracts/capabilities.md` covers `contributes.capabilities`: its six kinds, their
  fields and host consumers, validation, resolution and lifecycle, the two read routes, and how
  to add a kind.

### Changed
- `development/contracts/README.md` lists capability declarations among the contracts.

## 2026-09-29 13:05 — McHarbor 2026.10.01 — Contracts: how to use them, and all of them listed

Written against `src/backend/core/plugin/{contract,services,types,specs}/`, `shim.go`,
`src/backend/core/pluginruntime/{loader,abac_resources,desired_state_sources,incident_sources,external_login,cli_reserved}.go`,
`src/backend/core/notify/incident.go`, `src/frontend/core/plugins/`, and the plugins
`comm-slack`, `addon-reporting` and `addon-compute` for the examples.

### Added
- `development/contracts/plugin.md` — `Plugin` and its lifecycle order (routes publish only
  after `Init`), `Registry` and its three bands, `MenuRegistry`, `WidgetRegistry`,
  `NodeRegistry` with an executor example, `AccessRegistry`, the optional interfaces
  (`TabProvider`, `AdapterContributor`, `CLIProvider` with every CLI type, `SCIMHandler`), and
  the loader errors.
- `development/contracts/adapters.md` — the ten adapter families (spec, capabilities, client
  methods, optional extensions), the providers found by type assertion (`abac`,
  `desiredstate`, `incidentsource`, `incident`), and the compute types.
- `development/contracts/host-services.md` — three rules for every handle, all 21
  `HostContext` fields with their methods and nil-ability, per-handle notes, `ComputeRPC` by
  group, and the extension handles reached by type assertion.
- `development/contracts/types.md` — `Manifest` field by field, `Contributes`,
  `Capabilities`, and every declaration type with its Go shape.
- `development/contracts/tests.md` — the contract tests, moved out of the section index.

### Changed
- `development/contracts/README.md` — now a seven-step guide to using the contracts, an index
  of every contract linking to the pages above, and the rules for changing one.
- `development/hooks/README.md` — plugin routes sit under `/api/`, conventionally
  `/api/<slug>/…`, not always under the slug. Links into the new pages.
- `development/README.md`, `development/plugins/README.md`, `development/workflow/README.md` —
  row description and links.

## 2026-09-29 12:53 — McHarbor 2026.10.01 — Webhook secrets: the interface and the terminal

Written against `src/backend/modules/gitops/{routes,handler,webhook_secret}.go`, `src/frontend/modules/gitops/components/WebhookSecretPanel.tsx` and `src/cli/internal/cli/cmd_gitops_webhook_secret.go`.

### Added
- `gitops/webhook-secret/README.md` — what the secret authenticates, the GitOps page section, show-once behaviour, provider setup, rotate and remove, the CLI, the three routes.
- `cli/reference/mcharbor_gitops_webhook-secret*.md` — generated reference for `show`, `rotate` and `remove`.

### Changed
- `gitops/README.md` — index row for the new page.
- `cli/README.md` — Commands table row for `gitops webhook-secret`, with `gitops.view` and `gitops.manage`.
- `api/endpoints/gitops.md` — the three `/api/gitops/repos/{repoId}/webhook-secret` rows.
- `cli/reference/mcharbor_gitops.md` — regenerated; it now also lists `gitops preview`, whose reference pages were added alongside.

## 2026-09-29 12:35 — McHarbor 2026.10.01 — Development section limited to src and scripts/development

Written against `src/backend/core/plugin/{contract,specs,services}/`,
`src/backend/core/pluginruntime/`, `src/frontend/core/plugins/`,
`src/frontend/resources/hooks/`, the contract tests under `src/`, and `scripts/development/`.

### Changed
- `development/hooks/README.md` — now the extension hook points in `src/`: the backend
  registries, optional interfaces, adapter families and `HostContext`; the frontend mount
  points, their contexts, form slots and `window.McharborHost`; and the shared React hooks.
  Notes that `SCIMHandler` is declared but not dispatched.
- `development/contracts/README.md` — now the plugin contract (files, what the host enforces,
  why changes arrive as optional interfaces, CLI declarations) and the contract tests under
  `src/`.
- `development/workflow/README.md` — now only `src/` and `scripts/development/`: toolchain,
  source layout and conventions, every build script, `catalog.sh`, dev-data helpers, testing.
- `development/README.md`, `development/plugins/README.md`, `development/plugins/developing.md`,
  `README.md` — descriptions and rule statements no longer point outside `src/` and
  `scripts/development/`.

### Removed
- `development/assistant/README.md` and `development/quality/README.md` — out of scope for
  this section.

## 2026-09-29 12:24 — McHarbor 2026.10.01 — Development section

Written against `.claude/settings.json`, `.claude/hooks/*.sh`, `scripts/hooks/*.mjs` (and
`rules/`, `lib/`), `scripts/sanity/{gate.ps1,run-all.ps1,README.md}`, `Makefile`,
`scripts/lint/ruleguard/rules.go`, `src/frontend/package.json`, `.github/workflows/test.yml`,
`tests/run.ps1`, `.mcp.json`, `.claude/{rules,skills,agents,reference}/`, and the contract
tests in `src/backend/{api,core/rbac,core/apispec,core/pluginruntime,core/agent,core/db}/` and
`src/frontend/test/`.

### Added
- `development/README.md` — the section index, a six-step quick start, and pointers to the
  internal docs that stay outside this set (`.devdocs/testing/`, `.instructions/`,
  `.claude/reference/`).
- `development/workflow/README.md` — toolchain, repository layout, running and building every
  artifact, what a rebuild does not reach, the working rules, the test runner, promotion, and
  Windows traps.
- `development/hooks/README.md` — every Claude Code hook by event, with what it catches, whether
  it blocks and its timeout. Also the rule modules, the Stop/SubagentStop hand-back gate
  (file attribution, changed-line scoping, checks, reply lines, outcomes), the manual gate and
  its exit codes, local hooks, state files, and how to change a hook.
- `development/contracts/README.md` — every contract test: permissions, route gating and the
  OpenAPI document with its three generated consumers, architecture, plugins, the agent
  transport, migrations, the frontend floor, i18n, and the prose contracts.
- `development/quality/README.md` — linters and ruleguard rules, suppressions, the sanity
  suite with parameters, verdicts and checks, exemption markers, i18n tooling, and which CI
  steps block.
- `development/assistant/README.md` — where the Claude Code configuration lives, all 26 rules
  and when each loads, skills, agents with their models, MCP servers, reference pages, and what
  a reply owes.
- `development/plugins/README.md`, `development/add-ons/README.md` — indexes for the moved
  developer pages.

### Changed
- Moved every development page already in the set into `development/` (68 pages):
  `plugins/developing.md`, `plugins/architecture.md` and `plugins/authoring-cli/` →
  `development/plugins/`; `plugins/<family>/<slug>/developer.md` (55) →
  `development/plugins/<family>/<slug>.md`; `<add-on>/developer.md` (6) →
  `development/add-ons/<add-on>.md`; `workflows/developer/**` → `development/workflows/**`.
  284 links in 148 pages were repointed. No page's content changed apart from its links.
- `README.md` — a Development row in Sections and Start here. The Plugins row and the
  folder-layout example no longer list `developer.md`.
- `plugins/README.md` — a plugin folder now holds three pages, and the developer notes are
  linked from Development.

## 2026-09-29 02:10 — McHarbor 2026.10.01 — Plugin isolation and families

Written against `.claude/rules/plugin-authoring.md`, `src/cli/internal/cli/cmd_plugin_dev.go`
(`pluginSlugPrefixes`), `scripts/sanity/plugin-isolation/check.ps1`,
`scripts/sanity/duplication/{check.ps1,detect.mjs,accepted.json}`.

### Added
- `how-to/plugin/isolation-and-families.md` — scaffold instead of copy, the family slug
  prefixes, no cross-plugin imports and no plugin SDK (and why), `addon-*` as absolute, family
  bookkeeping in `accepted.json` with decisions D-1/D-2/D-4, naming every sibling on a family
  fix, the 0.03 drift check, and the contributor checklist.

### Changed
- `how-to/plugin/README.md` — the page table, "Where plugins live" and Related link to the new
  page.

## 2026-09-28 22:27 — McHarbor 2026.10.01 — Unknown database driver refused

Written against `core/dbconfig/dbconfig.go` (`Resolve`) and `core/config/config.go` (`DBConfig`).

### Changed
- `getting-started/configuration/README.md` — `MCHARBOR_DB_DRIVER` lists `mssql`; every accepted
  spelling; the startup error an unknown value now produces; `MCHARBOR_DB_DSN` names the SQL
  Server URL form.
- `getting-started/deployment/compatibility.md` — an unknown value stops the start, replacing the
  22:02 wording that it started on SQLite.
- `settings/system/high-availability/README.md` — the SQL Server section says the same.

## 2026-09-28 22:02 — McHarbor 2026.10.01 — SQL Server active/passive validated

Written against `core/config/config.go` (`DBConfig`), `core/coordinator/coordinator.go`,
`deploy/helm/mcharbor/templates/_helpers.tpl` and a `scripts/ha/failover-check.sh` run on
SQL Server 2022 (16.0.4295): takeover 4.6 s after a kill and 4.6 s after a clean stop.

### Changed
- `getting-started/deployment/compatibility.md` — SQL Server row: active/passive validated on
  2022, active/active readiness-gated; the active/passive paragraph names `sp_getapplock` and
  four backends; the SQL Server paragraph drops "single node", adds its two database
  requirements and the `MCHARBOR_DB_DRIVER=mssql` fix; the driver-value sentence now says an
  unknown value starts on SQLite without an error, which is what the code does.
- `getting-started/deployment/README.md` — both active/passive topologies list PostgreSQL,
  MySQL/MariaDB or SQL Server.
- `getting-started/deployment/kubernetes.md` — `externalDatabase.driver` takes `mssql`, and
  `mysql` is no longer described as single-mode only; the refusal row names the four backends.
- `settings/system/high-availability/README.md` — requirements, active lock and
  `externalDatabase` gate include SQL Server; new "SQL Server" section.
- `settings/system/database/README.md` — PostgreSQL and SQL Server card texts, and the intro
  sentence that called SQL Server single-node.

## 2026-09-28 16:46 — McHarbor 2026.10.01 — Plugin toolchain guard, demo vault

Written against `core/pluginruntime/toolchain.go` and `modules/demo/seed_vault*.go`. The
Go 1.27.1 version strings in `how-to/plugin/abi-and-libc.md` were already moved by the bump
itself.

### Changed
- `how-to/plugin/abi-and-libc.md` — new "A toolchain mismatch fails one plugin, not the
  host": why `plugin.Open` can no longer be trusted to reject a stale `.so` on Go 1.27, the
  build-info check that now runs first, the exact log and loader-status message, the
  `plugin has no Go build info` case, and that the boot repair does not fire on a toolchain
  bump because `built_for` records `VERSION`.
- `plugins/architecture.md` — "When a plugin does not load" gains the toolchain-mismatch row.
- `plugins/developing.md` — Debugging gains the same row.
- `settings/system/demo-mode/README.md` — new "Secrets vault" section: the sixteen seeded
  secrets by scope, why a small estate carries fewer, the stale / unused / never-revealed
  states it demonstrates, how values are derived and that reveal is still gated; a "What
  goes wrong" row for an older `demo.db`.

## 2026-09-28 11:42 — McHarbor 2026.10.01 — Runbook export, editing a schedule

The two pages the 03:40 entry recorded as owed rather than written blind. Both features have
shipped, so both are written against the code.

### Added
- `oncall/runbooks/export.md` — the three export formats and which permission each needs; the
  JSON file format field by field with a worked example; why `position` is not renumbered; what
  the file deliberately omits (ids, environment, author, service principal) and why a principal
  in particular cannot travel; the filename rule and the punctuation-only fallback; that the PDF
  and Word document are the `runbooks` report scoped to one runbook, stay in the report library,
  and never print a step's action parameters; the `runbook.exported` audit row; and the honest
  absence of both an import route and a `runbook export` CLI command.

### Changed
- `oncall/runbooks/README.md` — the list's table/card toggle, its search, and the readable
  schedule that search and sort actually read; a new "Exporting one" section pointing at
  `export.md`; `export.md` in Related.
- `settings/automation/schedules/README.md` — new "Creating and editing one": **Add Schedule**
  and **Edit schedule** open the same dialog, the per-field create/edit table, why **Active**
  is edit-only, that **Cron Expression** is the readable builder rather than a text box, the
  `schedules.manage` gate, and that editing exists so a schedule no longer has to be recreated
  at the cost of its execution history.
- `settings/automation/schedules/reference.md` — **Description** row added; **Status** is now
  **Active** and says it is edit-only; two new "When something does not behave" rows for the
  disabled edit button and the absent Active switch.

## 2026-09-28 01:55 — McHarbor 2026.10.01 — the runbooks report

### Changed
- `reporting/reports/README.md` — the `runbooks` type joins the assurance table and the
  sources table, gains a section explaining the scoped and unscoped documents and the PDF /
  DOCX export on the Runbooks page, and a line saying what it deliberately never prints.

## 2026-09-28 01:20 — McHarbor 2026.10.01 — instance backup destinations

### Changed
- `api/endpoints/instance-backups.md` — the create body takes `locationIds`, the export
  returns `destinations[]`, and the schedule bodies take `locationIds` in place of
  `locationId`. Says what the retained single-destination fields now summarise.
- `settings/system/instance-backup/schedules-and-verification.md` — **Also copy to** takes any
  number of locations; a run fails only when no destination accepted the export; saving
  replaces the whole set.
- `settings/system/instance-backup/export.md` — documents **Also copy to** and the **Copies**
  column on the manual export, and corrects **Deleting**, which still claimed a copy at a
  storage location was left alone. It has been removed with the backup since 2026-09-28.

## 2026-09-28 03:40 — McHarbor 2026.10.01 — Runbook export, schedule editing, offsite instance backups

### Changed
- No `.documentation/**` page changed in this pass. The runbook JSON export, the Runbooks
  card/table toggle, schedule editing in Settings → Automation, the offsite option on a manual
  instance backup, the Report Studio search and the tooltip work are all recorded in the root
  `CHANGELOG.md`; none of them contradicts an existing reference page, and the pages that would
  describe them do not exist yet.

### Note
- Owed, and tracked rather than written blind: a `runbooks/` folder documenting the export file
  format and the attachment kinds, and a line in `settings/automation/` for editing a schedule.
  Both are listed in `.references/todo/2026-09-28-runbook-document-export.md` and
  `.references/todo/2026-09-28-schedule-cli.md` so they are written against the shipped behaviour
  rather than from intent.

## 2026-09-28 01:55 — McHarbor 2026.10.01 — Plugin authoring: scaffold from the CLI

### Changed
- `plugins/developing.md` — § "Start from the skeleton" is now § "Scaffold it": a plugin is created with `mcharbor plugin dev new <slug>`, which writes it from a template embedded in the CLI binary rather than from a directory in the tree. Lists the slug families the packager accepts, and states that a plugin is not started by copying a sibling.
- `plugins/catalog.md` — § "Not in this list": the new-plugin template is no longer in the tree at all.
- `plugins/compatibility.md` — regenerated. The two `_example-skeleton*` rows are gone and the plugin count is 68.
- `plugins/authoring-cli/README.md` — the `plugin dev new` row describes the embedded template and no longer documents `--from`, which is removed.
- `how-to/plugin/README.md` — § "Where plugins live" points at the command instead of "copy it, rename the folder", with the reason a sibling is the wrong starting point.
- `cli/reference/mcharbor_plugin_dev_new.md` — synopsis and options match the new help text; the `--from` option is gone.

## 2026-09-27 20:40 — McHarbor 2026.10.01 — System Containers: restoring into another id range

### Changed
- `system-containers/backups/README.md` — new "Restoring into another id range": the archive holds host ids, the host re-owns the rebuilt tree into the target's range before the swap (lchown, mode and capability re-applied, ACLs and v3 capability root ids remapped, privileged to unprivileged and back), owners outside the source range refused, where the source range comes from (runs, container, manifest `idMap`, the tree itself), `err.ct.backup_idmap_conflict` (409), and the failed run from an agent that sends no ownership acknowledgement. The manifest row now lists `idMap`; five new "What goes wrong" rows, including what to do about containers restored across ranges by addon-lxc `2026.09.11` or older.
- `system-containers/troubleshooting.md` — row for the subuid capability item's old "room for N containers"; the migration list runs to `0004_backup_idmaps.sql`.

## 2026-09-27 19:36 — McHarbor 2026.10.01 — System Containers: dir-pool incrementals, console, stats

### Changed
- `system-containers/backups/README.md` — new "Why a dir pool's live copy is never an incremental" (tar matches directories by inode; the listing origin; the four full-copy reasons); the dir-pool row no longer claims large incrementals; restoring now covers the refusal of pre-fix dir-pool chains (`err.ct.backup_chain_unrestorable`, 409) and the recreate cleanup; three new "What goes wrong" rows.
- `system-containers/README.md` — new "The console and the init system": init detected from the rootfs, the inittab set up at create from an image, job control off on the console, exec sized 80×24.
- `system-containers/troubleshooting.md` — rows for the tty1–6 getty loop, "job control turned off", the stats fields, and the `memory.low` throttle.

## 2026-09-27 23:59 — McHarbor 2026.10.01 — Device path guard, VM passthrough holders, same-backend database move

### Changed
- `screens/containers/devices/README.md` — container paths are unique per container; the free-path suggestion and the warning.
- `virtual-machines/configuration.md` — `attachedTo` on host devices and the attach-dialog warnings.
- `settings/system/database/README.md` — **Move to another &lt;backend&gt; database** and the same-target refusal.

## 2026-09-27 22:50 — McHarbor 2026.10.01 — Agent releases from the McHarbor catalog, several versions side by side

### Changed
- `agents/releases/README.md` — the default source is the mirror `https://www.mcharbor.eu/catalog/agents/index.json`; a mirror may be named by its index file.
- `agents/releases/README.md` — the version list, **Other versions** → **Pull version**.
- `agents/releases/mirror-contract.md` — the index-file form, the `versions` field, `<version>/<build>/` folders with the flat fallback, and the `agent/catalog/` tree `publish-agent-catalog.mjs` writes.

## 2026-09-27 22:30 — McHarbor 2026.10.01 — SQL Server both ways, demo Git/GitOps, restore ownership badge

### Added
- `settings/system/demo-mode/README.md` — what demo mode seeds, including Git and GitOps, and what it keeps offline.

### Changed
- `settings/system/database/README.md`, `reference.md` — SQL Server as a target and as a source, the in-use backend, catch-up batch sizing, troubleshooting rows.
- `system-containers/backups/README.md`, `compute-foundation/troubleshooting.md` — Files-tab ownership inside a container rootfs; the restore ownership badge.
- `settings/system/README.md` — demo-mode index row.

## 2026-09-27 20:45 — McHarbor 2026.10.01 — Device picker, system containers verified on a live host

### Added
- `screens/containers/devices/README.md` — the host device picker: what is listed, the stable path it saves, permissions, the `host.view` key.

### Changed
- `system-containers/backups/README.md`, `system-containers/troubleshooting.md` — full backups for running dir-pool containers and why; refused legacy chains; restoring into another id range; console, shell and busybox init; stats; the start and staging behaviour.
- `screens/containers/README.md`, `fields.md`, `troubleshooting.md` — the devices section and its permission row.

## 2026-09-27 17:00 — McHarbor 2026.10.01 — Runbook plugin steps, LDAP sign-in, System Containers fixes

### Added
- `oncall/runbooks/plugin-steps.md` — every plugin-backed runbook step with its route and permission keys.

### Changed
- `oncall/runbooks/README.md` — the editor dialog, selectors, attaching to plugin objects.
- `plugins/identity/ldap/**`, `identity/README.md`, `plugins/developing.md`, `screens/security/identity-setup/README.md` — LDAP sign-in through `POST /api/identity-ldap/auth`, host-applied lockout/MFA/session, the corrected scopes.
- `system-containers/configuration.md`, `system-containers/backups/README.md` — the `/api/system-containers` prefix; create from image, delete, and the backup error hint.

## 2026-09-27 14:35 — McHarbor 2026.10.01 — Host services, host update recovery, database fixes

### Added
- `screens/environments/services/README.md`, `screens/environments/services/troubleshooting.md` — the Services tab: fields, keys, protected units, the drop-in model, routes, the agent build it needs, and what goes wrong.
- `settings/updates/host-os/README.md` — Host OS updates for the machine McHarbor runs on, how it reaches that host, and the keys.

### Changed
- `screens/environments/README.md` — "When a package cannot be downloaded", with the per-manager table for Skip unavailable packages and the automatic retry.
- `api/endpoints/host.md`, `settings/updates/README.md` — the self-host routes and the new option.
- `settings/system/database/README.md`, `reference.md` — values above 2 GiB on PostgreSQL/MySQL, migration 119, the automatic SQLite optimize.
- `getting-started/deployment/compatibility.md`, `getting-started/deployment/README.md` — SQL Server row, active/active readiness-gated.
- `screens/environments/services/*` — the agent build floor is `202609271429`.

## 2026-09-27 13:40 — McHarbor 2026.10.01 — Agent releases, unattended runbooks, GitOps deploys, debug shell, recreate, HA streams, database validation

### Added
- `agents/releases/README.md`, `agents/releases/mirror-contract.md` — the agent release store, verification, settings, serving precedence and the mirror layout.
- `oncall/runbooks/unattended.md`, `screens/security/service-principals/README.md` — unattended runs and service principals.
- `gitops/promotions/README.md` — how a promotion checks out, deploys and records its outcome.
- `screens/containers/debug-shell/README.md` — the debug shell, BusyBox provenance and cleanup.
- `getting-started/deployment/content-security-policy/README.md`, `getting-started/deployment/security-posture/README.md`, `screens/security/mfa-enforcement/README.md` — indexed from their parents.

### Changed
- `agents/**`, `getting-started/installation/**`, `settings/**/agents` — installs and updates only from McHarbor, no GitHub fallback.
- `gitops/README.md`, `gitops/previews/README.md` — previews deploy the pull request; the forge probe.
- `oncall/runbooks/README.md` — unattended runs exist.
- `system-containers/backups/README.md` — recreating a deleted container; manifest row corrected.
- `settings/system/high-availability/active-active.md`, `high-availability/README.md` — what is relayed between nodes.
- `getting-started/deployment/compatibility.md`, `getting-started/deployment/README.md`, `deployment/kubernetes.md` — SQL Server row, MySQL/MariaDB active/passive validated, active/active readiness-gated, where `deploy/` and `packaging/` differ.
- `cli/README.md` — `agent releases`, `settings`, `runbook automate|principals`, `gitops pipeline promote --wait`, `container exec --debug`, `lxc backup recreate`.
- `screens/containers/**`, `api/endpoints/containers.md` — debug-shell routes and permission.

## 2026-09-27 12:20 — McHarbor 2026.10.01 — Dir-pool container backups without a freeze

### Changed
- `system-containers/backups/README.md` — a running dir-pool container is copied with no freeze and recorded as **Live copy**, which is not point-in-time; consistency table, dir row and the dir paragraph rewritten.

## 2026-09-27 11:55 — McHarbor 2026.10.01 — Compliance report as Word and PDF

### Changed
- `screens/enterprise/compliance.md` — **Refresh** and **Download report** are in the page header; the report format is JSON, Word (.docx) or PDF (.pdf), printable HTML removed, and what the PDF does with characters Helvetica cannot draw.

## 2026-09-27 11:45 — McHarbor 2026.10.01 — Internet-facing hardening, attestations, container-backup snapshots

### Added
- `getting-started/deployment/security-posture/README.md` — the boot-time `security posture:` warnings, each with the variable that silences it, and what an internet-facing install needs regardless of code.
- `getting-started/deployment/content-security-policy/README.md` — the default policy, its two deliberate relaxations, `CONTENT_SECURITY_POLICY` / `CONTENT_SECURITY_POLICY_REPORT_ONLY`, and the SAML POST-binding exception.
- `screens/security/mfa-enforcement/README.md` — `security.mfa_required` (off / admins / all), what a covered user sees, which routes stay reachable, and recovering a locked-out administrator.
- `compliance/attestations/README.md` — fields, the two routes, refusal reasons, `compliance.attest` versus `compliance.manage`, and what an attestation does not freeze.

### Changed
- `compliance/soc2/README.md`, `compliance/iso-27001/README.md`, `compliance/evidence-integrity/README.md` — the review-record rows move from *not covered* to *evidenced*, naming the attestation routes.
- `compliance/README.md` — index entry, permission and route rows for attestations.
- `api/settings/reference.md`, `api/conventions/authentication.md`, `api/endpoints/auth.md` — the three `security.lockout_ip_*` settings and per-address lockout.
- `system-containers/backups/README.md` — per-backend snapshot table (btrfs, ZFS, LVM thin, dir), leftover cleanup, three new failure rows.
- `cli/README.md` — `compliance attest` / `compliance attestations` rows, `--allow-unsigned` on `plugin install|update`, stale "not yet registered" note removed.
- `plugins/architecture.md`, `plugins/overview.md` — unsigned-install consent is server-enforced.
- `getting-started/deployment/README.md`, `getting-started/README.md`, `getting-started/configuration/README.md`, `screens/security/README.md` — index entries and env rows for the new pages.
- `getting-started/prerequisites/README.md`, `settings/system/database/README.md` — "active-active" corrected to multi-node active/passive, the mode that exists.

## 2026-09-27 04:45 — McHarbor 2026.10.01 — Plugin CLI commands, system-container backups, and the roadmap run

### Added
- `plugins/cli-commands/README.md` — the authoring page for the plugin→CLI contract: the optional
  interface, the seven interaction kinds and what each requires, flags and dotted body paths, root
  naming and the names a plugin cannot have (aliases count), the permission-agreement rule and why
  an undeclared route cannot satisfy it, the three risk levels and why a plugin does not choose its
  own confirmation rules, how the spec reaches an operator, and a table of what goes wrong.
- `system-containers/backups/README.md` — what an artifact contains, where it goes, the two
  permission keys, restore modes, the quiesce rule and what happens when neither mechanism is
  available.
- `getting-started/deployment/compatibility.md` — the compatibility matrix: version against
  database backends, agent floors, HA modes, plugin store modes and the Docker and Kubernetes
  versions tested against. The agent floor rows are read from the constants, so the historical
  gates are quoted rather than rewritten.
- `compliance/` — a new section with an index and four folders: `cis-docker-benchmark/`, `soc2/`,
  `iso-27001/` and `evidence-integrity/`. The last one is the important one: it states where
  McHarbor's audit evidence is weaker than it sounds, control by control.
- `oncall/runbooks/README.md` and `incidents/runbooks/README.md` — the runbook engine from the two
  sides an operator meets it from, plus `oncall/offline.md`.
- `gitops/previews/README.md` — preview environments, including the two limits stated plainly: a
  webhook now needs a secret, and promotion does not yet deploy into the environment it creates.
- `plugins/authoring-cli/README.md` — the local `plugin dev` toolchain, with a section of its own on
  why it has no permission keys.
- `cli/reference/` — seven regenerated command pages.

### Changed
- `cli/README.md` — a *Plugin commands* section replacing the claim that a plugin's capabilities are
  unreachable from `mcharbor`: the six roots with their aliases and real permission keys, the two
  scoping notes that differ from the rest of the CLI (`k8s` is cluster-scoped, and its thirteen
  commands are thirteen because the plugin registers thirteen routes), a warning that `ct` and `ctr`
  mean different things, and rows for the Support, Plugin authoring, Interactive screen, Capacity,
  Runbook, Compliance and Cluster HA groups. The *Not in this release* section was rewritten: it had
  claimed support bundles had no server endpoint, which stopped being true when `modules/support`
  landed.
- `plugins/README.md` — indexes the two new plugin pages.
- `getting-started/deployment/kubernetes.md` — the long-lived WebSocket and SSE paths enumerated by
  name, and a note that Docker Compose remains the supported default.
- `getting-started/deployment/README.md` — links to the compatibility matrix.
- `oncall/README.md` — corrected: it said the on-call view added no permission keys, which the
  runbook engine changed.
- `system-containers/README.md` — backups added to the index, permissions and migrations.

## 2026-09-27 04:26 — McHarbor 2026.10.01 — Compliance packs: evidence for a control framework

### Added
- `compliance/README.md` — the new section index: what a compliance pack is, its three parts
  (mapping, check runner, report), the three verdicts a control can carry, the verdict
  vocabulary, the permission keys, the routes as mounted, retention, and what a pack does not do.
- `compliance/cis-docker-benchmark/README.md` — the mapping for the first framework: 38 controls,
  31 checked mechanically, 2 evidenced, 5 not covered, with the evidence source named per row and
  the two thresholds the pack chose stated so a reader can disagree with them.
- `compliance/soc2/README.md` — the SOC 2 evidence mapping. No check runner; CC6.3's review
  requirement is recorded as **not covered**, because no `reviewed_at`, `reviewed_by` or
  `last_reviewed` column exists anywhere in McHarbor's schema.
- `compliance/iso-27001/README.md` — the ISO/IEC 27001 Annex A evidence mapping, with clauses 9.2
  and 9.3 recorded as not covered for the same reason.
- `compliance/evidence-integrity/README.md` — what the audit log's tamper-evidence does and does
  not prove, per configuration: the chain is a keyed HMAC and seals nothing when
  `MCHARBOR_SECRET` is unset, the export "signature" is an HMAC under the instance's own key and
  cannot be verified by an external auditor, the export is capped at 10,000 rows, write-once
  ships off with a sanctioned-delete bypass, and forwarding ships as a plugin so a plain install
  forwards nothing. Ends in a checklist to run before citing the log as evidence.

### Changed
- `README.md` — a Sections row for the new Compliance section, and a "Produce evidence for an
  audit" row in *Start here*.
- `cli/README.md` — a Commands row for `compliance packs` / `runs` / `results` / `run`, with
  `compliance.view` and `compliance.manage`, noting that the command file is not yet registered
  in `root.go`.

## 2026-09-27 02:51 — McHarbor 2026.10.01 — Reporting: sixteen new report types and the report catalog

### Added
- `reporting/reports/README.md` — the report catalog: every built-in and contributed type with
  its levels and period, what the ten governance and resilience types read and which
  permission each needs, what those reports never print, what the newer contributed types
  cover, and how option labels resolve.

### Changed
- `reporting/README.md` — where report types come from (original set, two families, contributing
  plugins); backend size; catalog added to the section index.
- `reporting/developer.md` — source layout regenerated; new "Adding a report type" section
  covering the original set, report families, and the conventions for contributed types
  (namespaced label keys, `reports.options.<key>`, accepted status words, `SourceStatus`).
- `virtual-machines/developer.md`, `system-containers/developer.md`,
  `compute-foundation/developer.md` — the new `reports_*.go` files in the source table.
- `virtual-machines/README.md`, `system-containers/README.md`, `compute-foundation/README.md`,
  `plugins/logging/siem/README.md`, `plugins/logging/syslog/README.md` — link to the report
  catalog.
- `plugins/logging/siem/configuration.md`, `plugins/logging/syslog/configuration.md` —
  `reporting.adapter.register` in the host scopes table.
- `plugins/logging/siem/developer.md`, `plugins/logging/syslog/developer.md` — the delivery
  report provider, its permission pair, and why a failed registration does not stop the plugin.

## 2026-09-27 02:09 — McHarbor 2026.10.01 — Reconciler: plugin kinds and selectors

### Added
- `plugins/desired-state/README.md` — the `desiredstate` contract, which plugins implement it
  and with which properties, the enforcement rules, and what goes wrong.

### Changed
- `screens/reconciler/README.md` — plugin kinds in **What to watch**; the form's selectors,
  **Enter manually…**, **Not managed** and **Use current values**.
- `plugins/README.md` — index row.
- `api/endpoints/gitops.md`, `api/endpoints/README.md` — the five `/api/reconciler/kinds` routes,
  the plugin fields of `POST /api/reconciler`; counts.

## 2026-09-27 01:43 — McHarbor 2026.10.01 — VM stop reasons and guest health

### Changed
- `incidents/README.md` — Virtual Machines signals: unexpected stops, guest-initiated
  shutdowns, guest frozen, disk full inside the guest, clock drift; the lifecycle actions and
  QEMU events in the evidence; the host agent requirement.
- `plugins/incident-sources/README.md` — the Virtual Machines signal list.

## 2026-09-27 01:19 — McHarbor 2026.10.01 — Notifications search and filters

### Changed
- `screens/notifications/README.md` — search, the All / Unread / Read chips and the level
  filter, all server-side.
- `api/endpoints/monitoring.md`, `api/endpoints/README.md` — the list's `q`, `mode`, `level`,
  `status` parameters and `GET /api/in-app-notifications/counts`; counts.

## 2026-09-27 01:15 — McHarbor 2026.10.01 — Audit coverage: uninstalled plugins, and a section that described the wrong screen

### Changed
- `screens/audit/README.md` — Coverage figures corrected to the four the page renders
  (`<n>% covered`, Routes requiring audit, Mapped to an action, Unmapped); removed an
  "Events" figure and a quoted sentence that come from a locale key no component reads;
  named `audit_action_route_map` as the register behind the numbers; new **Plugin activity**
  subsection on the Uninstalled marker and why a removed plugin keeps its row.
- `screens/audit/fields.md` — plugin events carry a last-event date, an event count and an
  **Uninstalled** marker.
- `screens/audit/troubleshooting.md` — "Check the Overview tab" replaced (the page has no
  Overview tab); new row for a removed plugin still listed under Coverage.

## 2026-09-27 01:09 — McHarbor 2026.10.01 — Plugin incident sources, CLI self-signed toggle

### Added
- `plugins/incident-sources/README.md` — the `incidentsource` contract: subject types, signals,
  evidence sections, recovery, and which plugins implement it.

### Changed
- `incidents/README.md` — incidents raised by the KVM, LXC, Kubernetes and Swarm plugins.
- `plugins/README.md` — index row for incident sources.
- `api/endpoints/incidents.md`, `api/endpoints/README.md` — `GET /api/incidents/sources`; counts.
- `cli/README.md` — the **Server uses a self-signed certificate** toggle.

## 2026-09-27 00:55 — McHarbor 2026.10.01 — Container stats, shared streams, table selection

### Added
- `agents/stats/README.md` — push and pull container stats: the four paths and their cost,
  the **Container stats** card and `GET /api/metrics/stats-path` (`host.view`), how push
  works (one batch in flight, the offline-buffer fallback, dedup, backpressure), the
  **Stats push interval** setting, the version gate (2026.09.01 build 202609251653), updating
  a native agent by hand, and what goes wrong.

### Changed
- `agents/README.md`, `README.md` — index entries for the new page.
- `agents/configuration/README.md`, `settings/system/agent/README.md`,
  `settings/system/agent/reference.md` — **Metrics collection** defaults to **On** (it
  said Off; `DefaultAgentSettings` turns it on); the ~2s per container
  cost applies only to agents that cannot push; new **Stats push interval** row; removed "the
  background collector skips agent environments".
- `agents/protocol/README.md` — new *Container stats* section: `stats_subscribe`,
  `stats_push`, `stats_push_ack`.
- `agents/operations/README.md`, `agents/troubleshooting/README.md`,
  `agents/use-cases/README.md`, `screens/containers/troubleshooting.md`,
  `screens/environments/troubleshooting.md` — symptoms and fixes rewritten around push
  versus pull.
- `api/conventions/streaming.md` — the agent stats constraint now names the push path, and
  `GET /api/events/stream` is documented as one shared Docker subscription per environment.
- `screens/table-views/README.md` — new *Selecting rows* (**Select all N matching**, not on
  server-paged tables) and *Rows per page* (saved with the layout and applied on open).
- `screens/README.md` — the shared *Lists* row mentions **Select all N matching**.
- `screens/environments/README.md` — the Overview tab lists the **Container stats** card.

## 2026-09-27 00:48 — McHarbor 2026.10.01 — Endpoint reference completed

### Added
- `api/endpoints/governance.md` — the approval queue and rules, break-glass accounts and
  sessions, `POST /api/governance/simulate`, `GET /api/governance/conflicts`, and the two
  access-review exports (23 routes).
- `api/endpoints/secrets.md` — the secrets vault: metadata, rotation, references, apply,
  rollback and the step-up-gated reveal (14 routes).
- `api/endpoints/settings.md` — the settings API route table, including
  `PUT /api/settings/branding` and the public `GET /api/branding` (27 routes).
- `api/endpoints/incidents.md` — incidents, findings, narration settings, ITSM channels and
  links, and the public `POST /api/incident-webhooks/{token}` (19 routes).
- `api/endpoints/instance-backups.md` — instance export, key file, schedules, verification,
  upload, plan and restore (17 routes).
- `api/endpoints/database.md` — the active database, connection test, health check and repair,
  migration and cleanup (11 routes).
- `api/endpoints/support.md` — diagnostics, the encryption-key source, the state inventory,
  the pre-upgrade check and support bundles (10 routes).

### Changed
- `api/endpoints/README.md` — the seven new pages; Incidents and Settings now point at their
  endpoint pages; every count recomputed from the tables (724 endpoints across 28 pages).
- `api/endpoints/auth.md` — `POST /api/auth/sudo` with its scopes, and the seven
  `/api/table-views/*` routes.
- `api/endpoints/environments.md` — the public agent binary download, agent probe,
  prerequisite install, redeploy, saved SSH access and health history, environment
  duplicates, merge and connection migration (13 routes).
- `api/endpoints/monitoring.md` — the audit denials and AI trails, audit retention, the log
  forwarder routes, and channel adapter capabilities (13 routes).
- `api/endpoints/system.md` — cluster readiness, startup health, failover, drain and resume,
  `GET /api/livez`, `GET /api/readyz` and `GET /api/updates/agents`; the cluster status and
  HA migration plan rows now name `cluster.view`, which the routes enforce.
- `api/endpoints/host.md` — `GET /api/metrics`, `GET /api/metrics/stats-path`, host OS info,
  and the OS update state, schedule and stream; the OS update check and apply rows now name
  `host_updates.view` and `host_updates.manage`, which the routes enforce.
- `api/endpoints/security.md` — scanner adapters and scanner instance configuration (6 routes).
- `api/endpoints/identity.md` — identity provider endpoint URLs, mapping preview and SAML
  metadata import; SCIM dry-run and token rotation.
- `api/endpoints/users.md` — bulk account actions, bulk delete and the lifecycle report.
- `api/endpoints/backups.md` — provider plans and provider runs.
- `api/endpoints/api-keys.md` — `POST /api/api-keys/{id}/rotate`.
- `api/endpoints/store.md` — `POST /api/appstore/plugins/{slug}/reconcile`.
- `api/endpoints/networks.md` — `GET /api/networks/host-interfaces`.
- `api/endpoints/ai.md`, `api/endpoints/roles.md` — the endpoint count in the opening line
  now matches the table.

## 2026-09-27 00:35 — McHarbor 2026.10.01 — Endpoint reference and proxy trust

### Changed
- `api/endpoints/users.md` — `GET` / `PUT /api/elevations/elevatable`, the `PUT` body, and
  `permissionLabel` on request rows.
- `api/endpoints/monitoring.md` — `GET /api/alerts/template-reference` and
  `POST /api/alerts/template-preview` with its body.
- `api/endpoints/roles.md` — `GET /api/rbac/policy-reference`.
- `getting-started/configuration/README.md`, `getting-started/deployment/reverse-proxy.md` —
  `MCHARBOR_TRUSTED_PROXY_CIDRS` now governs login throttling, the request log, audit rows,
  password-reset records and `request.ip`, not only API-key usage.

## 2026-09-27 00:31 — McHarbor 2026.10.01 — Alert message templates

### Changed
- `settings/monitoring/alerts/reference.md` — new *Message templates* section: every placeholder
  including the new `{{operator}}`, `{{subjectId}}`, `{{environmentId}}`, `{{alertId}}`,
  `{{time}}`, `{{date}}`, `{{timestamp}}`; per-condition `{{ctx.*}}` values; the reference panel,
  the preview and its two routes.

## 2026-09-27 00:10 — McHarbor 2026.10.01 — Policies, elevations

### Added
- `screens/security/policies/README.md` — how a policy decides at the route gate, the resource
  types from the host and the KVM, LXC and Compute plugins, the permissions picker, every
  variable and function in the expression reference, and what goes wrong.
- `screens/security/elevations/README.md` — requesting and deciding, the routes, and the rule
  for which permissions are requestable (admin choice, legacy flag, default rule).

### Changed
- `screens/security/README.md` — the policies permission row names `rbac.policies.view` /
  `rbac.policies.manage` (it named no key); `users.manage` row covers elevations; both new
  pages indexed.

## 2026-09-27 00:10 — McHarbor 2026.10.01 — Governance, identity, audit center, supportability

### Added
- `screens/security/governance/` — `README.md`, `api-key-rotation.md`, `approvals.md`, `break-glass.md`, `role-templates.md`, `simulator.md`, `user-lifecycle.md`: approval rules and
  requests, the policy simulator, role templates, user lifecycle and bulk actions, break-glass,
  API key rotation.
- `screens/security/scim/README.md`, `screens/security/identity-setup/README.md`,
  `screens/security/access-review/README.md` — SCIM dry run and sync, the provider setup wizard
  with metadata import and the role impact preview, the Effective access tab and catalog export.
- `screens/audit/rbac-denials.md`, `ai-action-trail.md`, `retention.md`, `integrity.md` — the
  denial and AI trail tabs, retention by severity with sealed tombstones, and chain verification.
- `settings/system/support/` — `README.md`, `support-bundle.md`, `diagnostics.md`,
  `state-inventory.md`, `pre-upgrade-check.md`.

### Changed
- `screens/security/README.md` — Governance group, approvals keys, the new pages indexed.
- `screens/audit/README.md`, `actions.md`, `fields.md`, `troubleshooting.md` — tabs, actor and
  severity filters, export columns.
- `plugins/identity/README.md` — links to the setup wizard.
- `settings/system/README.md` — Supportability row.

## 2026-09-27 00:10 — McHarbor 2026.10.01 — HA readiness, startup health, node identity, MySQL runtime

### Changed
- `settings/system/high-availability/README.md` — MySQL 8.4 and MariaDB 11 listed as supported
  shared databases, with a *MySQL and MariaDB* section on the driver-level translation and the
  skipped migration statements; new *Readiness checklist*, *Startup health* and *Node identity*
  sections; `MCHARBOR_HA_MODE=active-active` documented as refused; the two new routes under
  `cluster.view`; MySQL/MariaDB removed from *What is not supported*.

## 2026-09-26 23:51 — McHarbor 2026.10.01 — Enterprise packaging, compliance and environment reference

### Added
- `getting-started/deployment/` — `README.md`, `compose.md`, `kubernetes.md`, `reverse-proxy.md`: topologies, the hardened and external-database Compose files, the Helm chart and generated manifests, Caddy/Traefik/NGINX in front of one or two nodes.
- `getting-started/verifying-releases/README.md` — what the release workflow signs, `cosign verify`, `cosign verify-attestation --type spdxjson`, `cosign verify-blob` for SBOM files and `SHA256SUMS`.
- `screens/enterprise/` — `README.md` (the Enterprise sidebar section and its keys), `compliance.md` (every section, its measurements and verdicts, `GET /api/compliance/posture`, `compliance.view`, the report dialog), `environment-variables.md`.
- `plugins/compatibility.md` — generated matrix of every bundled plugin's `mcharbor:` range against the current and previous version.

### Changed
- `screens/README.md` — Enterprise routes and sidebar section; the "withdrawn" note removed.
- `getting-started/README.md`, `README.md` — Deployment and Verifying releases rows.
- `getting-started/installation/README.md` — reverse-proxy section links the worked configurations.
- `plugins/README.md` — Compatibility row.
- `api/endpoints/system.md` — `GET /api/compliance/posture` and `compliance.view`.

## 2026-09-26 23:20 — McHarbor 2026.10.01 — High availability

### Added
- `settings/system/high-availability/README.md` — active/passive on PostgreSQL: requirements, the four settings, how a node decides its role, what a standby refuses, Make active / Drain / Resume, split brain, plugin package stores, the probes, and what is not supported.

### Changed
- `settings/system/README.md` — High availability row.
- `getting-started/configuration/README.md` — `MCHARBOR_HA_MODE`, `MCHARBOR_PLUGIN_STORE`, `MCHARBOR_ADVERTISE_URL`, `MCHARBOR_RESET_KEY_CHECK`; `MCHARBOR_NODE_ID` now names the node in the topology and audit log.

## 2026-09-26 21:56 — McHarbor 2026.10.01 — ITSM tickets and ticketing plugins

### Added
- `settings/monitoring/alerts/tickets/README.md` — the ticket lifecycle, link states, the outbox and when it stops retrying, status coming back, the inbound route and its protection, routes and keys.
- `plugins/communication/{pagerduty,alertmanager-webhook,zammad,glpi,itop,otrs,request-tracker}/` — README, configuration and troubleshooting for each ticketing plugin.
- `workflows/nodes/integrations/README.md` — Open Ticket, Update Ticket, Resolve Ticket; `workflows/nodes/triggers/README.md` — Ticket Status.

### Changed
- `settings/monitoring/alerts/README.md` — Tickets section and index row.
- `settings/integrations/communications/README.md` — "Ticket status from the ITSM", reserved `_` keys, 18 plugins.
- `plugins/communication/README.md` — "Ticketing and on-call" table; permission key shape corrected to `channels.use_comm_<name>`; Test behaviour qualified.
- `plugins/catalog.md` — the seven ticketing plugins.
- `workflows/README.md`, `workflows/nodes/README.md` and the category and developer pages — 90 built-in nodes.

## 2026-09-26 20:38 — McHarbor 2026.10.01 — Incidents and narration

### Added
- `incidents/README.md` — what opens and resolves an incident, the view, the two permission keys.
- `incidents/narration.md` — opting in, what is sent and removed, the answer's shape, attribution, refusals, on-call.
- `incidents/behaviour.md` — the four detectors and their thresholds.
- `incidents/reference.md` — the twelve routes and the audit events.

### Changed
- `README.md` — Incidents section and a Start here row.
- `api/endpoints/README.md` — the reconciler count (43) and an Incidents row.
- `oncall/README.md` — the narrative on container alerts.

## 2026-09-26 20:03 — McHarbor 2026.10.01 — Drift plans, enforcement and Git sources

### Added
- `screens/reconciler/plans-and-enforcement.md` — plan steps and manual reasons, approving and its re-check, the enforcement conditions, the breaker, the audit events.
- `screens/reconciler/git-sources.md` — document format, syncing rules, Git-managed watches, accepting drift as a pull request.

### Changed
- `screens/reconciler/README.md` — kinds, responses, the three permission keys including `reconciler.enforce`.
- `screens/reconciler/actions.md`, `fields.md`, `troubleshooting.md` — rewritten for responses, plans, Git sources and settings.
- `api/endpoints/gitops.md` — 43 endpoints; *Answering drift* and *Desired state from Git*; `networks`, `capAdd`, `capDrop`, `healthcheck` config keys; the loop is no longer described as observe-only.

## 2026-09-26 19:20 — McHarbor 2026.10.01 — Terraform workflows, plugins and inventory

### Changed
- `terraform/resources.md` — new `mcharbor_workflow` and `mcharbor_plugin` sections (attributes, routes, permission keys, import); data sources `mcharbor_containers`, `mcharbor_images`, `mcharbor_app_catalog`, `mcharbor_plugin_catalog`; three troubleshooting rows.
- `terraform/README.md` — Status now covers Phase 3.

## 2026-09-26 19:01 — McHarbor 2026.10.01 — Scheduled self-updates

### Changed
- `settings/updates/mcharbor/README.md` — updates never land in the middle of work: the check and install schedules, deferral and skipped windows.
- `settings/updates/mcharbor/reference.md` — *Look for a new release*, *Install automatically in this window*, *Keep the window open for (minutes)*; *Check interval* removed; two new troubleshooting rows.

## 2026-09-26 18:25 — McHarbor 2026.10.01 — Instance backup and restore

### Added
- `settings/system/instance-backup/README.md` — what an export holds and leaves out, the four permission keys.
- `settings/system/instance-backup/export.md` — the key (passphrase or key file), the file format, download with step-up, routes.
- `settings/system/instance-backup/restore.md` — the three modes, the plan, refusals, the order of a restore, interruption, the audit anchor and its approval, `mcharbor restore --local`, routes.
- `settings/system/instance-backup/schedules-and-verification.md` — restore rehearsals, schedules, retention, storage copies, the widget, the two alerts, routes.

### Changed
- `settings/system/README.md` — Instance backup row.
- `cli/README.md` — `restore --local` moved out of *Not in this release*; `audit verify` anchors.
- `cli/reference/` — regenerated.

## 2026-09-26 15:11 — McHarbor 2026.10.01 — agent keeps the CLI link

### Changed
- `agents/installation/README.md` — a native agent creates the `mcharbor` link at start-up and after a self-update.

## 2026-09-26 15:00 — McHarbor 2026.10.01 — CLI in the agent binary

### Changed
- `agents/installation/README.md` — the install script links `mcharbor` to the agent; manual install and uninstall lines for the link.
- `cli/README.md` — install row "On an agent host".

## 2026-09-26 14:31 — McHarbor 2026.10.01 — CLI container exec

### Changed
- `cli/README.md` — `container exec` in the command table with its two permission paths.
- `cli/troubleshooting.md` — exec exit status, the terminal requirement, Git Bash path conversion, protected containers.
- `cli/reference/` — regenerated; adds `mcharbor_container_exec.md`.

## 2026-09-26 14:11 — McHarbor 2026.10.01 — CLI audit search and plugin commands

### Changed
- `cli/README.md` — `audit search` and its filters; `plugin install/update/uninstall`; *Not in this release* updated.
- `cli/reference/` — regenerated.

## 2026-09-26 14:05 — McHarbor 2026.10.01 — Drift loop Phase 2

### Changed
- `api/endpoints/gitops.md` — the drift report route and page, the *Desired state drifted* alert condition, the Drift widget.

## 2026-09-26 13:59 — McHarbor 2026.10.01 — Push notifications in each user's language

### Changed
- `oncall/push.md` — notifications follow the recipient's language, then the instance default.

## 2026-09-26 13:57 — McHarbor 2026.10.01 — Audit export key, device login browser

### Changed
- `api/endpoints/monitoring.md` — `GET /api/audit/export` needs `audit.export`.
- `cli/README.md` — `audit export` needs `audit.export`; `login --device` opens the browser, `--no-browser`.

## 2026-09-26 13:26 — McHarbor 2026.10.01 — Drift loop Phase 1

### Changed
- `api/endpoints/gitops.md` — what a desired state can watch (container, stack, environment), how compose is read, the typed `config`, drift classes and `firstSeenAt`, and the observe-only refusal on reconcile.

## 2026-09-26 13:12 — McHarbor 2026.10.01 — CLI device sign-in, CLI downloads, more command groups

### Changed
- `cli/README.md` — install from your McHarbor (*Security → API keys → Command-line client*); signing in with `login --device` and its exit codes; ten new rows in the commands table; the checksum file is `mcharbor_<version>_checksums.txt`; *Not in this release* rewritten for what remains.
- `cli/reference/` — regenerated from the binary (74 new pages).
- `api/endpoints/auth.md` — device sign-in routes (`/api/device/*`), 27 endpoints.
- `api/endpoints/system.md` — CLI download routes (`/api/cli/*`), 31 endpoints.
- `api/endpoints/README.md` — 531 endpoints; Authentication and System rows.

## 2026-09-26 12:40 — McHarbor 2026.10.01 — Pool file delete refuses open files

### Changed
- `compute-foundation/troubleshooting.md` — the in-use refusal from the pool file browser, and how to clear it.

## 2026-09-26 12:27 — McHarbor 2026.10.01 — Trusting the root on a phone

### Changed
- `oncall/install.md` — Android and iOS steps for installing McHarbor's root CA.
- `oncall/push.md` — the background-worker row names the browser's reason and links the steps.

## 2026-09-26 12:16 — McHarbor 2026.10.01 — Push troubleshooting

### Changed
- `oncall/push.md` — *What goes wrong* gains the background-worker message shown when the service worker cannot start.

## 2026-09-26 03:46 — McHarbor 2026.10.01 — On-call phone view and Web Push

### Added
- `oncall/README.md` — the on-call section: the five screens and the key each needs, reaching it, the layout choice, deep links, what stays on the desktop.
- `oncall/install.md` — the manifest, icons and service worker; the trusted-certificate requirement; Android and iOS install; updates.
- `oncall/push.md` — what a push carries and through whom, who receives what, turning it on, the device list, failure handling, the instance VAPID keys.
- `oncall/offline.md` — what is cached and how staleness is shown, what is never cached, sessions that expire in the background.
- `api/endpoints/push.md` — the manifest, icon and `/api/push` routes.

### Changed
- `README.md` — On-call row in Sections and in Start here.
- `api/endpoints/README.md` — Push and install page; 524 endpoints across 22 pages.
- `getting-started/configuration/README.md` — `MCHARBOR_PUSH_CONTACT`.

## 2026-09-26 03:20 — McHarbor 2026.10.01 — API key attribution, connection redaction, approvals

### Changed
- `cli/README.md` — Audit section: rows now name the API key and seal its id; GitOps row gains `gitops approval list/approve/reject` and `gitops.approve`.
- `screens/audit/fields.md` — the User column's "via API key <name>" line and the details' API key row.
- `api/endpoints/environments.md` — connection details are returned only to `environments.manage` holders.
- `api/endpoints/gitops.md` — approve and reject take `{note}`; the resolver is always the caller.

## 2026-09-25 23:15 — McHarbor 2026.10.01 — The mcharbor command-line client

### Added
- `cli/README.md` — new section: install (five platform archives, the CI image, the copy inside the McHarbor container), first use, contexts and where keys are stored, every flag and environment variable, the command groups with the permission keys each needs, how audit attribution works, and what is not in this release.
- `cli/scripting.md` — the frozen exit codes, output formats, the JSON error object, the reversible / disruptive / irreversible confirmation rules, following long-running operations and log streams, a CI example.
- `cli/troubleshooting.md` — certificates, 401 and 403, not found, confirmation refusals, version skew, clock, `doctor`.
- `cli/reference/` — 79 pages generated from the binary with `mcharbor docs markdown`.

### Changed
- `README.md` — Command-line client added to Sections and Start here.

## 2026-09-25 19:35 — McHarbor 2026.10.01 — Query timing, agent spans, agent RPCs, drift loop

### Changed
- `observability/README.md` — rows for `mcharbor_db_query_*`, `mcharbor_agent_rpc_*`, `mcharbor_agent_spans_relayed_total` and the four `mcharbor_reconciler_*` families; "What is not measured yet" replaced by "What is measured where".
- `observability/tracing.md` — the agent's own `agent.proxy` and `docker` spans, why they are relayed rather than exported, clock skew, sampling, the agent version needed, the bounded upload queue.
- `observability/troubleshooting.md` — reconciler lag that keeps rising, and agent spans missing from a trace.
- `api/endpoints/gitops.md` — the background drift loop (observe only, unknown-not-drifted, two-pass confirmation, audit actions) and the drift call's new error on an unreachable environment.
- `getting-started/configuration/README.md` — `MCHARBOR_RECONCILE_INTERVAL`.

## 2026-09-25 18:01 — McHarbor 2026.10.01 — Observability: Prometheus metrics and OTLP traces

### Added
- `observability/README.md` — new section: the `GET /api/metrics` endpoint, its `metrics.scrape` key (scope `global`), the no-I/O guarantee, every metric family with its type and labels, and what is not measured yet (query duration, reconciler lag, agent RPCs other than the Docker proxy).
- `observability/scrape-config.md` — creating a scrape-only API key, the Prometheus job with TLS against McHarbor's own root, the HA note that leader-only work reports from the leader, and importing `packaging/grafana/mcharbor-control-plane.json`.
- `observability/tracing.md` — the OTLP/HTTP variables, off by default, what each span covers, and how `traceparent` crosses the agent hop to the Docker daemon.
- `observability/troubleshooting.md` — the 401/403 codes a scrape can get, x509, empty panels, telling a wedged agent from a slow one, a mass plugin failure, a collector that receives nothing.

### Changed
- `README.md` — Observability added to Sections and Start here.
- `getting-started/configuration/README.md` — `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` rows.

## 2026-09-25 13:55 — McHarbor 2026.10.01 — TLS versions: preferred vs minimum

### Added
- `getting-started/installation/README.md` — a **TLS versions** section: TLS 1.3 as the preferred and shipped floor, TLS 1.2 as the minimum still supported, the exact env line and restart for each direction, the two `openssl s_client` commands that prove which is in force, and the note that a terminating reverse proxy governs what clients actually negotiate.

### Changed
- `getting-started/configuration/README.md` — the `TLS_MIN_VERSION` row now labels the default as preferred, names TLS 1.2 as the minimum accepted, lists both sets of spellings, and links the new section.

## 2026-09-25 13:40 — McHarbor 2026.10.01 — TLS 1.3 is the default floor

### Changed
- `getting-started/configuration/README.md` — the `TLS_MIN_VERSION` row now reads `772` (TLS 1.3) as the default, lists the 1.3 spellings, and says when to drop to `1.2`.
- `getting-started/installation/README.md` — the TLS paragraph now states the 1.3 default and names the clients that need `1.2`: OpenSSL 1.0.2, Java 10 and earlier, TLS-inspecting middleboxes.

## 2026-09-25 13:35 — McHarbor 2026.10.01 — Testing moves to the internal docs

### Removed
- `testing/` — the Testing section (`README.md`, `running.md`, `writing-frontend.md`,
  `writing-backend.md`, `contracts.md`) is developer-only and now lives in
  `.devdocs/testing/`, unchanged.

### Changed
- `README.md` — Sections and Start here no longer list Testing.

## 2026-09-25 13:08 — McHarbor 2026.10.01 — The migration files

### Added
- `settings/system/database/reference.md` — *The migration files*: where the files live,
  how `schema_migrations` records them, and what each of `065_settings.sql` through
  `081_license.sql` adds.

### Changed
- `settings/system/database/reference.md` — the verified set is 82 files; the skipped-on-external-
  drivers table names `072_audit.sql` and `067_rbac.sql` and gains the API-key archive trigger
  in `080_api_keys.sql`.
- `api/settings/reference.md` — the scanner settings copy is attributed to `079_scanners.sql`.
- `testing/writing-backend.md` — the pinned-count example no longer quotes file counts.
- `settings/system/database/README.md` — the Reference row names the migration files.

## 2026-09-25 12:13 — McHarbor 2026.10.01 — Testing section current; Terraform section

### Added
- `terraform/README.md`, `terraform/resources.md` — the Terraform provider in
  `src/terraform-provider-mcharbor/`: provider settings, every resource and data source, import
  IDs, what lands in state, and the permission key each operation needs.

### Changed
- `testing/running.md` — the four-backend migration run, the Terraform provider, redocly validation
  and the live `GET /api/docs` (with `?source=core`), `unrecorded-routes.txt`, the runnable
  Playwright suites (`check:a11y`, `test:visual`, `MCHARBOR_API_TARGET`), runner i18n coverage,
  every CI step plus the `migrations` job, and an empty known-failures list.
- `testing/writing-frontend.md` — *Destructive dialogs: the shared pattern*: `MutationConfirmDialog`,
  `ConfirmDialog`'s `permission` prop, and `describeDestructiveDialog`.
- `testing/contracts.md` — the unrecorded-route contract, the option table (`Request`/`Response`,
  `Scoped`, `Paginated` with `per_page` up to 1000, stream kinds), and plugin routes in the live
  document.
- `testing/README.md` — the `test-floor` rule, and the layer table brought up to date.
- `README.md` — Sections and Start here gain the Terraform section.

## 2026-09-25 11:32 — McHarbor 2026.10.01 — Testing section

### Added
- `testing/README.md` — index: every test layer, where it lives, its runner, and whether CI runs it.
- `testing/running.md` — commands for backend, agent, frontend, the contract set, regenerating
  `api/openapi.json` with `MCHARBOR_EMIT_OPENAPI`, the root `tests/test.cmd` runner and its
  staging, the Playwright suites (not installed), the CI steps, and the known failures.
- `testing/writing-frontend.md` — how to create a frontend test: placement and staged-name
  collisions, the `src/frontend/test/` helpers, the four assertions for a destructive control,
  permissions, MSW, i18n, Radix portals, timing, property tests, and the test floor.
- `testing/writing-backend.md` — how to create a backend test: layout, SQLite fixtures, the
  recording router, plugin-linked packages to leave alone, and the toolchain flags.
- `testing/contracts.md` — the permission and route-gating contracts, each assertion, its fix, and
  when `apispec.Exempt` is allowed.

### Changed
- `README.md` — Sections and Start here gain the Testing section.

## 2026-09-25 01:25 — McHarbor 2026.10.01 — Templates, VM backup actions, default layouts, macvlan in the overview pages

### Added
- `virtual-machines/backups.md` — *Actions*: the per-row actions on the machine's Backups
  tab and the host's Backups page (Back up now, Edit, Delete on a schedule; Restore,
  Delete on a backup; Open from the Backups page), their keys, the deep link into the
  machine's tab, and that Download and Cancel are not offered for machines.
- `screens/table-views/README.md` — *The shipped default layouts*: 23 tables open on a
  shipped layout until the user saves one, and Reset to default returns to it.

### Changed
- `virtual-machines/README.md` — *How it works* gains making a template (button placement,
  confirm, `vm.manage`, stopped only) and creating a machine **From a template** (linked or
  full copy, clone route and its audit action).
- `backup/screen/actions.md` — virtual machine rows carry the plugin's own actions and keys.
- `screens/table-views/README.md` — Reset to default now returns to the shipped layout.
- `compute-foundation/getting-started.md` — "Three modes" becomes four, with the macvlan
  row and when to pick it.
- `compute-foundation/README.md` — the Guest networking index row names macvlan and the
  no-address diagnosis.

## 2026-09-25 01:18 — McHarbor 2026.10.01 — Guest network troubleshooting

### Added
- `compute-foundation/guest-networking.md` — the **macvlan** mode: parent interface, VLAN
  tag, the four link modes (`bridge`, `vepa`, `private`, `passthru`) and what each allows
  between guests, host access and its address (prefix must match the LAN, never an address
  in use, no address means no reach), and the four macvlan refusals.
- `compute-foundation/guest-networking.md` — *A guest gets no address*: a four-step
  diagnosis from one `tcpdump` on the parent (request not sent / not answered / answered
  but not received), hypervisor MAC filtering, Wi-Fi, port security and VLANs, reading a
  macvtap's RX counters, the `txqueuelen 0` fault and its live fix, the
  `MCHARBOR_VM_VHOST=off` test, checking which process holds the tap, guest-side causes,
  and a symptom table for host reachability, guest-to-guest traffic and leftover
  interfaces.
- `agents/configuration/README.md` — *Virtual machines* section documenting
  `MCHARBOR_VM_VHOST`.

### Changed
- `how-to/agent/README.md` — *Undoing an override*: `sudo systemctl revert mcharbor-agent`
  followed by `sudo systemctl restart mcharbor-agent`, why the restart is needed, and that a
  VM-shaping variable reaches a guest only on its next stop and start.
- `agents/configuration/README.md` — the same revert-and-restart commands replace the
  one-line revert note.
- `compute-foundation/guest-networking.md` — "three modes" becomes four; the Docker macvlan
  section points to macvlan mode as the route that leaves the host alone.
- `compute-foundation/troubleshooting.md` — rows for a guest with no DHCP address, the
  macvtap RX-drop signature and a macvlan guest that cannot reach its host.
- `virtual-machines/troubleshooting.md` — rows for a VM with no IP and an agent setting
  that did not take effect.

## 2026-09-24 17:45 — McHarbor 2026.10.01 — Table views

### Added
- `screens/table-views/README.md` — personal, server-stored table views:
  the Edit table dialog (Shown list, column settings, Add column, Reset to
  default), drag-and-keyboard column resizing, the saved-filters menu (save,
  apply, update, rename, default, delete), the `/api/table-views/*` routes
  and their no-permission-key status, and the one-time browser import.

### Changed
- `screens/README.md` — points to the new page from the "Lists" section.

## 2026-09-24 00:54 — McHarbor 2026.10.01 — Virtual machine restore

### Changed
- `virtual-machines/backups.md` — restore is available; the "not available yet"
  note is replaced by a *Free space on the host* section stating that a restore
  needs roughly twice the machine's disk size free, why (the current disks stay
  untouched until the rebuilt ones are complete) and that the agent refuses up
  front rather than failing mid-transfer.

## 2026-09-24 00:32 — McHarbor 2026.10.01 — Virtual machine backups

### Added
- `virtual-machines/backups.md` — scheduled off-host backups for machines: how
  the bytes travel and why McHarbor stages nothing, mandatory encryption with
  the key never leaving the host, full versus incremental and every event that
  invalidates a change log, the host-side check that stops an incremental being
  recorded that was not taken, scheduling, retention and the rule that a
  backup an incremental depends on is never pruned, the three guards on
  restore, the permission per action, and a symptom table. States plainly that
  restore is not available on this build yet.

### Changed
- `virtual-machines/README.md` — the new page in the section index.

## 2026-09-23 21:21 — McHarbor 2026.10.01 — QEMU features

### Added
- `virtual-machines/qemu-features.md` — the feature set McHarbor can ask QEMU for
  beyond processors, memory and a disk: the capability gate and what it probes,
  resizable memory, processor hotplug, hugepages, NUMA placement, virtiofs shared
  directories, the direct host channel, per-disk I/O limits, shared disk access,
  `qemu-img` diagnostics, the virtual security key, the `io-error` and
  `guest-panicked` run states, and what the guest agent is used for. Carries the
  permission key per control and a symptom table.

### Changed
- `virtual-machines/README.md` — the new page in the section index, and a line
  under *How it works* saying the host's QEMU is probed rather than assumed.

## 2026-09-23 18:25 — McHarbor 2026.10.01 — The compute host detail page

### Added
- `compute-foundation/host-detail.md` — the new per-host page: the Overview, Settings,
  Tests and Logs tabs, the permission each control needs, the eight log sources and what
  each one reads on the host, the agent build the last three require, and what goes wrong.

### Changed
- `compute-foundation/README.md` — the new page is listed in *In this section*, and three
  stale counts in *What it contributes* are corrected: routes 22 → 43, backend 9 files /
  ~4,250 lines → 21 files / ~8,270 lines, and the migration list now names all five.

## 2026-09-23 17:51 — McHarbor 2026.10.01 — Host updates: how far an upgrade may go

### Changed
- `screens/environments/README.md` — the *Updates* tab paragraph now names the two
  permission keys that gate it (`host_updates.view` to read, `host_updates.manage` to
  schedule or apply), and gains a *How far an upgrade is allowed to go* section. It explains
  why a Debian or Ubuntu kernel is reported as **kept back**, what the new **Install new
  packages when required** setting does, that it defaults to off and removes nothing, and
  that it affects apt only — `dnf`, `apk` and `pacman` already install new dependencies,
  and `zypper` is left alone on purpose because its only equivalent is a distribution
  upgrade.
- `screens/environments/troubleshooting.md` — three rows: kernel packages still listed after
  a successful apply, an unchanged kernel version until the host is rebooted, and the
  setting appearing to do nothing against an agent binary that predates it.

## 2026-09-23 02:20 — McHarbor 2026.10.01 — Alpine is not a compute host either

### Changed
- `compute-foundation/host-requirements.md` — the `### Alpine` package block is gone. It
  sat among the per-distribution install blocks as if it were an option, with the
  disclaimer underneath it; a reader scanning for their distribution found a command to
  run. It is replaced by the plain statement that Alpine is not supported as a guest host,
  matching the wording `getting-started/prerequisites/README.md` and
  `installation/docker-engine.md` already use.
- `compute-foundation/host-requirements.md` — Alpine dropped from the distributions
  `install-host-dependencies.sh` is described as mapping, and `shadow-uidmap` dropped from
  the system-container subordinate-id tooling list. The script still carries those names;
  naming them here implied a host the probe refuses.
- `compute-foundation/supported-platforms.md` — same correction to the mapped-distribution
  sentence under *Recommended guest host*, with the reason stated once: OpenRC rather than
  systemd, which the probe refuses.

Alpine as a **guest** is untouched and remains correct: the image-catalog coverage list and
the Alpine 3.21 system-container root filesystem in `supported-platforms.md`, the
`alpine:3.20` backup helper image in `workflows/nodes/actions/README.md`, the root-CA
install instructions in `screens/security/install-root-ca.md`, and the remote agent's
deliberate static-musl build target in `how-to/plugin/abi-and-libc.md`.

## 2026-09-23 01:05 — McHarbor 2026.10.01 — Add-ons and backup become their own sections

### Added
- `backup/README.md` — a new top-level **Backup** section. The chain from what is backed up
  through consistency, schedule, destination, encryption and retention to a four-layer
  restore, the full permission table, and the index of everything backup touches: the screen,
  a container’s own settings, the storage-destination and snapshot-driver plugins, the two
  settings sections, the six workflow action nodes and the 44 API endpoints.
- `plugins/addons.md` — the former `plugins/addons/README.md`, kept in the plugin section as
  the mention-and-link page for the six add-ons: the table, what they have in common as
  plugins, the compute family, and Reporting hosting an adapter family of its own.

### Changed
- **The six add-ons are now top-level sections**: `virtual-machines/`, `system-containers/`,
  `kubernetes/`, `docker-swarm/`, `reporting/` and `compute-foundation/` moved out of
  `plugins/addons/`. Page contents are unchanged; every relative link in them was resolved
  against the old location and re-emitted against the new one. “In this folder” is now
  “In this section”, and each “Related” block points at `plugins/addons.md` rather than the
  deleted family index.
- `screens/backups/` moved to `backup/screen/` — the screen, its actions, its fields and its
  troubleshooting table, unchanged, now inside the section they belong to.
- `README.md` — the Sections table gains **Backup**; a new **Add-ons** table lists the six
  sections; the Start-here table gains four rows; the folder-layout example shows
  `backup/screen/` and `virtual-machines/`.
- `plugins/README.md` — states the one exception to “one folder per plugin”, renames the
  Feature-plugins row to **Add-ons** noting each has its own section, and points the storage
  and snapshot rows at the Backup section for what uses them.
- `plugins/catalog.md`, `plugins/overview.md` — add-on rows repointed at the new sections;
  “Feature plugins” reads “Add-ons” where it is navigation rather than architecture.
- `plugins/storage/README.md`, `plugins/snapshots/README.md` — **these two families stay in
  the plugin section**; their Related blocks now link to Backup and to the settings section
  where a destination or driver is configured.
- `screens/README.md` — the `/backups` route row points at `backup/screen/README.md` and
  names the Backup section.
- Inbound links repointed in `agents/installation`, `agents/prerequisites`,
  `getting-started/first-run`, `getting-started/prerequisites`, `how-to/README.md`,
  `how-to/reporting`, `api/endpoints/backups.md`, `screens/containers`, `screens/volumes`,
  `screens/dashboard`, `screens/environments`, `settings/automation`,
  `settings/general/themes`, `settings/integrations/{README,storage,snapshots}` and
  `settings/monitoring/data-retention`.

### Removed
- `plugins/addons/README.md` — replaced by `plugins/addons.md` one level up, now that the
  six folders it indexed are sections in their own right.

## 2026-09-23 00:00 — McHarbor 2026.10.01 — No `.apk`, anywhere

### Changed
- `getting-started/installation/README.md` — the native install formats no longer
  include `.apk`. No workflow builds one: `release.yml` packages `deb` and `rpm` only.
- `getting-started/configuration/README.md` — same correction in the Native install
  lead-in, and `MCHARBOR_INSTALL_KIND` is documented as `deb`, `rpm` or `tarball`
  rather than `deb`, `rpm` or `apk`.
- `getting-started/installation/native-linux.md` — "All four artifacts" was left over
  from when the `.apk` was one of them; there are three. Also removed the blank gap the
  earlier `apk add` deletion left in the per-format install block, and corrected the
  install-kind values.

Every other Alpine mention in the set was checked and is correct as it stands: Alpine as
a *guest* OS and system-container rootfs (`compute-foundation/supported-platforms.md`),
as the backup helper *image* (`workflows/nodes/actions/README.md`), as a machine that
*trusts* the McHarbor root CA (`screens/security/install-root-ca.md`), as the remote
agent's deliberate static-musl build target (`how-to/plugin/abi-and-libc.md`), and in
`compute-foundation/host-requirements.md`, where the package block is followed by the
statement that it is listed for completeness and is not a supported guest host.
`prerequisites/README.md`, `installation/docker-engine.md` and
`installation/native-linux.md` already said Alpine is not supported.

## 2026-09-22 22:52 — McHarbor 2026.10.01 — Packaging page matches the tooling again

### Changed
- `how-to/plugin/packaging.md` — **Build** rewritten. It described `make list` /
  `build` / `sign` / `lint` / `clean` under `src/plugins/Makefile`, which delegates to
  `Plugins/*/Makefile`. No such Makefile exists in this repository, so those targets
  iterate over an empty list and report success having built nothing. The page now
  documents `scripts/development/build-plugin.sh <slug> <catalog-url>`, that the version
  comes from `plugin.yaml` rather than an argument, that the catalog URL is not a write
  target, and `build-plugins.sh` for a whole release.
- `how-to/plugin/packaging.md` — **Sign** rewritten. The `make sign-index` /
  `verify-index` block referenced `signing/key.pem`, which does not exist. Signing is
  part of `build-plugin.sh`, through `sign-plugin.py`, and the page now shows the PyNaCl
  virtualenv and `MCHARBOR_PYTHON` that it actually needs.
- `how-to/plugin/packaging.md` — **Publish** rewritten. The catalog is `index.json`
  plus per-plugin entries, `files/` and `icons/`, not a signed `index.yaml`;
  `parseRemotePayload` reads JSON only and nothing in the host parses YAML. The
  `output/` layout, the index step and its drift warnings, and the publish-before-you-
  release ordering are now written down, and `src/plugins/index.yaml` is marked
  vestigial.
- `how-to/plugin/packaging.md` — **The developer loop** corrected. It told the reader to
  run `make build` in the plugin folder and then rebuild the image; `docker/Dockerfile`
  no longer builds or bundles plugins, so an image rebuild delivers nothing. The loop is
  build → copy into `mnt/plugin-catalog` → **Update** in the Plugin Store, which is the
  only path that moves all four values the loader cross-checks.

## 2026-09-22 22:29 — McHarbor 2026.10.01 — Plugin ABI traps and which key signs

### Changed
- `how-to/plugin/abi-and-libc.md` — the ABI-hash table gains the input that names a
  package the plugin never mentions: a shared dependency the plugin's own `go.mod`
  moves. `go get` will downgrade a module the host pins, and the resulting `.so` fails
  `plugin.Open` naming `golang.org/x/sys/cpu`. The packager now holds every host pin
  and logs each one.
- `how-to/plugin/abi-and-libc.md` — new *What it does not protect* under `built_for`:
  it records a version string, so an ABI change that leaves `VERSION` alone is
  invisible to the repair pass. The Alpine→Debian move is the worked example — every
  installed `.so` was musl-linked and skipped, because `built_for` still matched.
- `how-to/plugin/abi-and-libc.md` — new *Two traps worth recognising* under building:
  a CRLF `go.mod` putting a carriage return inside a rewritten import path (only
  plugins with Go sub-packages reach the rewrite, and the printed path looks correct),
  and a payload that omits `migrations/` installing cleanly and then failing one table
  at a time.
- `how-to/plugin/packaging.md` — the *Sign* section named no key. It now states that
  the host trusts `MCHARBOR_PLUGIN_TRUST_KEYS` (`/app/trust/plugin-signing.pub`, from
  the root `signing/` pair), that the unrelated `src/plugins/signing/` pair must not be
  used, that a mismatch installs quietly as unsigned, and lists the three carriers a
  signature can arrive by.

## 2026-09-22 01:45 — McHarbor 2026.10.01 — Clock drift reference

### Changed
- `api/settings/reference.md` — the `audit.clock.reference_url` default is now
  `https://www.cloudflare.com/`. worldtimeapi.org stopped resolving, so every
  drift check recorded `reason=reference_unavailable` and drift was never
  actually measured; the check only reads the response's `Date` header, so any
  reachable host that answers `HEAD` with a 2xx serves.

## 2026-09-22 01:05 — McHarbor 2026.10.01 — No auto environment, and the two host checks

### Changed
- `getting-started/first-run/README.md` — step 1 rewritten: McHarbor creates no environment
  on a fresh install, so the first one is made by hand, and until it exists no page has
  anything to show and no metrics are collected.
- `getting-started/README.md` — the five-minute path no longer tells the reader to confirm
  a local environment that is never created.
- `getting-started/installation/README.md` — the socket mount's "losing it means" corrected
  now that no environment is created for it.
- `screens/environments/README.md` — states that nothing is created for you, and documents
  the two read-only host checks in the create dialog (**Check Docker**, **Check VM
  readiness**): what each runs, which items block, and why neither gates the Add button.
- `agents/prerequisites/README.md` — points at the two checks as the way to answer the
  compute requirements before committing to a host.
- `plugins/addons/compute-foundation/host-requirements.md` — new *Installing the VM
  libraries by hand* section: the four packages that matter, the command for Debian/Ubuntu,
  RHEL/Fedora, SUSE, Arch and Alpine, the module load and `kvm` group step no package
  performs, and the subordinate-id tooling system containers need instead.

## 2026-09-22 00:52 — McHarbor 2026.10.01 — Docker Engine install, and the ABI contract

### Added
- `getting-started/installation/docker-engine.md` — installing Docker Engine on Debian
  12/13, Ubuntu 22.04/24.04/25.04, Fedora 38+ (including the dnf5 subcommand change),
  RHEL/Rocky/Alma 9, openSUSE and Arch, and states that Alpine is not supported at all.
  Notes that the glibc 2.36 floor
  constrains only the *native* McHarbor package and not the container, so a host below it
  runs the container normally. Covers removing Ubuntu's conflicting `docker.io`,
  the `docker` group's root-equivalence, verifying with `docker compose version`, checking
  the socket path via `docker context inspect`, and what rootless changes.
- `how-to/plugin/abi-and-libc.md` — why a plugin only loads into the host build it was
  compiled against: what `plugin.Open` compares and the six inputs that move the hash, why
  `core/pluginruntime` is split from `core/plugin`, the base image of every build stage,
  why the remote agent stays static musl on Alpine, why `go.work` must not return to the
  host build, and how `built_for` and the boot-time repair pass behave.

### Changed
- `getting-started/installation/README.md` — runtime base corrected from `alpine:3.20` to
  `debian:bookworm-slim`, the stale "and the bundled plugins" build-stage claim removed,
  and the new Docker Engine page added to the folder index.
- `getting-started/prerequisites/README.md` — Docker Engine row links the new install
  page; runtime image described as Debian-based rather than Alpine-based.
- `how-to/plugin/README.md`, `how-to/plugin/packaging.md` — both index and link the new
  ABI page; "Changing something shared" now points at the full account.
- `README.md` — the layout example lists the two new pages.

## 2026-09-22 00:27 — McHarbor 2026.10.01 — Plugins move to the remote catalog

### Changed
- `getting-started/installation/native-linux.md` — removed the `.apk` from the format
  list, the `apk add` install command, the `apk del` purge row and the "Alpine's gap"
  section, since Alpine is no longer a native target; replaced the musl note with the
  statement that every artifact links glibc. Added a "Plugins on a native install"
  section covering the empty payload, the outbound HTTPS requirement to
  `https://www.mcharbor.eu/catalog/plugins`, the tarball-upload path for air-gapped
  hosts, and the post-upgrade repair pass with its **Needs repair** state and
  `plugins.manage` gate.
- `how-to/plugin/packaging.md` — build scripts moved from `scripts/plugins/` to
  `scripts/development/`, and `build-signed-plugin.sh` is now `build-plugin.sh`.

## 2026-09-21 16:50 — McHarbor 2026.10.01 — Native Linux install

### Added
- `getting-started/installation/native-linux.md` — a new page documenting the native
  install this release ships alongside the container: the `.deb`, `.rpm`, `.apk` and
  tarball formats and their roughly 1.1 GB size, the glibc 2.36 floor and the exact
  `GLIBC_2.36 not found` failure on Ubuntu 22.04/20.04, Debian 11 and Rocky/RHEL/Alma 9
  (stated without softening the RHEL 9 exclusion), the `/opt/mcharbor` /
  `/var/lib/mcharbor` / `/etc/mcharbor/mcharbor.env` layout, upgrading and the automatic
  plugin re-sync, the `rpm -e` purge asymmetry with the manual cleanup commands, the
  `docker`-group root-equivalence statement, two troubleshooting entries, and Alpine's
  missing OpenRC service file. Sibling to the existing `installation/troubleshooting.md`.

### Changed
- `getting-started/installation/README.md` — a **Container or native** section pointing at
  the new page; the container stays the documented default. New page linked from "In this
  folder".
- `getting-started/configuration/README.md` — a new **Native install** table for
  `MCHARBOR_INSTALL_KIND` (set automatically by the postinstall, not hand-edited), plus
  `MCHARBOR_PLUGIN_CATALOG_ROOT` and `MCHARBOR_THEME_CATALOG_ROOT` added to **App Store and
  plugins**, with a note that `MCHARBOR_PLUGIN_CATALOG_ROOT` is distinct from the
  pre-existing `MCHARBOR_PLUGIN_CATALOG_DIRS`, which drives the plugin-icon fallback.
- `getting-started/prerequisites/README.md` — a new subsection under **Access to the
  Docker socket** stating the native install's equivalent: the `mcharbor` service account's
  `docker`-group membership is root-equivalent on any host running a Docker daemon, the
  same fact the container's socket mount already carries here, expressed through a group
  instead of a mount. Related list gains the new page.

### Note
- The brief for this documentation asked for `.documentation/application/{native-install.md,
  deployment-and-runtime.md, configuration-and-data.md, auth-and-security.md}`. That whole
  `application/` folder no longer exists in this reorganised set, so the content above was
  retargeted to the nearest existing pages instead of recreating deleted ones: the
  container-vs-native comparison went into `installation/README.md`, the configuration
  content into `configuration/README.md`, and the security statement into
  `prerequisites/README.md`, which already carries the container's own version of the same
  statement.

## 2026-09-21 12:43 — McHarbor 2026.10.01 — The database health check

*(Stamped with the workstation clock. The three entries below it read 12:13–15:00 for the
same day; that spread is a clock discrepancy recorded in the 2026-09-21-1156 journal, not
an ordering error introduced here.)*

### Added
- `settings/system/database/health-check.md` — a new page for a feature that shipped with no
  documentation at all: the four check families and what each one can and cannot see per
  driver, why a skip always states its reason, how the orphan check discovers unconstrained
  relations and reports what it could not map, why an index a later migration dropped is not
  "missing", the four remedies and their fixed order, the re-verification before a
  destructive repair, and the governed-table rule that keeps `audit_logs` out of every
  delete path.

### Changed
- `settings/system/database/README.md` — `database.manage` now also names running the check
  and applying a repair; the check is listed under what the tab governs; the new page is in
  the folder index.

## 2026-09-21 12:43 — McHarbor 2026.10.01 — An audit row outlives what it names

### Changed
- `screens/audit/README.md` — new **The trail outlives what it names** section: the
  environment and entity names are stored on the row at write time rather than resolved
  against the current list, so a deleted environment still reads by name and free-text
  search matches the name as it was when the event happened; and `audit_logs` is a governed
  table whose dangling references the database health check reports but never offers to
  prune.
- `screens/audit/fields.md` — the Entity Name and Environment columns record that the name
  shown is the stored one.

## 2026-09-21 12:43 — McHarbor 2026.10.01 — Picking a macvlan parent interface

### Changed
- `screens/networks/fields.md` — Parent interface is a selector over the host's own
  physical, bond and VLAN links rather than a text box, each listed with kind, address and
  MTU, with an **Enter manually** escape; each driver mode carries a one-line description.
  New **Where the interface list comes from** section: why McHarbor's own container cannot
  answer this, the agent and already-in-use sources, what is filtered out and why a bridge
  is among them, and why a socket environment opens straight into manual entry.

## 2026-09-21 12:43 — McHarbor 2026.10.01 — What creating a guest network does to the host

### Changed
- `plugins/addons/compute-foundation/guest-networking.md` — new **What the form tells you
  before you submit** section: the dialog now lists the ordered host-side steps per mode and
  what deleting the network undoes. For external mode it records the 45-second rollback the
  agent arms before it replies — so a long pause, a generic error and a briefly offline
  agent is that rollback working — and the two caveats it does not remove: IPv6 addressing
  is not carried onto the bridge, and the host's own network manager is not told the address
  moved.

## 2026-09-21 15:00 - McHarbor 2026.09.01 - Changing environment variables on an agent

### Added
- `how-to/agent/README.md` - a task-shaped guide for reconfiguring a remote agent: the
  read-once rule, identifying the install, the systemd drop-in / unit / `EnvironmentFile`
  routes, the no-supervisor case, recreating a `docker run` container, the Compose and
  `.env` route with the repository service's `MCHARBOR_AGENT_*` indirection, how to confirm
  from the log that the process was actually replaced, and the changes that need more than
  a restart.

### Changed
- `how-to/README.md` - the Guides table carries the new agent guide.
- `agents/configuration/README.md` - Related points at the new how-to.

## 2026-09-21 12:13 - McHarbor 2026.09.01 - Agent transport security on the Environments list

### Changed
- `screens/environments/fields.md` - the Connection column now carries a shield for agent
  environments, green for an encrypted control channel and orange for plaintext or
  verification-disabled, with the exact mode on hover.
- `agents/protocol/README.md` - the `auth` step records the new `tlsMode` field, its four
  agent-reported values, and the `encrypted` fallback McHarbor derives from the request
  scheme for an agent too old to send one.

## 2026-09-21 13:45 - McHarbor 2026.09.01 - MCHARBOR_PIN names the root CA, and the switch controls it

### Changed
- `screens/security/https.md` - the Remote agents section is rewritten. The pin names the
  **root CA**, not the served certificate, because the leaf is reissued every 90 days and a
  pin on it would drop every agent at each renewal. Records that the agent builds a real
  chain to the pinned anchor and checks the hostname, that a leaf fingerprint is rejected
  with a message saying so, and that turning the self-signed switch off emits no pin at all.
- `agents/configuration/README.md` - `MCHARBOR_PIN` is documented as the root CA's
  fingerprint, with the renewal-survival reason.
- `agents/installation/troubleshooting.md` - points at the root CA rather than the server
  certificate.

## 2026-09-21 13:05 - McHarbor 2026.09.01 - Every install method pins, not just the script

### Changed
- `screens/security/https.md`, `agents/configuration/README.md` - the copy-paste Docker,
  Compose, binary and systemd snippets in the agent dialog are built in the frontend and
  were still emitting only `MCHARBOR_INSECURE`. They now carry `MCHARBOR_PIN` too, so the
  documentation no longer describes the script and the SSH deploy as the only pinned paths.

## 2026-09-21 12:40 - McHarbor 2026.09.01 - Agent installers pin instead of skipping verification

### Changed
- `screens/security/https.md` - the Remote agents section now describes what the installers
  do on their own: `MCHARBOR_PIN` set from the served certificate, the root carried inline
  and passed to the script's own `curl` as `--cacert`, and `MCHARBOR_INSECURE` emitted as
  `false`. Adds why no pin is emitted for an uploaded certificate.
- `agents/installation/README.md` - the unit-file note points at `MCHARBOR_PIN` rather than
  `MCHARBOR_INSECURE`, and says the installer writes it.
- `agents/configuration/README.md` - records that the install script and the SSH deploy set
  the pin automatically.

## 2026-09-21 11:58 — McHarbor 2026.10.01 — A local CA replaces the bare self-signed certificate

### Added
- `screens/security/https.md` — the certificate model end to end: what the four files under
  `$DATA_DIR/tls/` are, how to verify a fingerprint out of band through `docker logs`, how to
  install the root per platform, what **Regenerate** does and does not rotate, `MCHARBOR_PIN`
  for remote agents, and why HSTS waits until the root is installed.

### Changed
- `screens/security/README.md` — the HTTPS line now mentions the fingerprint and the root CA
  download; the folder index gains the new page.
- `screens/security/actions.md` — **Download root CA**, **Regenerate** and **Copy fingerprint**
  added to the action table. The upload row no longer claims a container restart is needed:
  the reload watcher hot-swaps within 30 seconds.
- `screens/security/fields.md` — the fingerprint and root CA read-only fields.
- `agents/configuration/README.md` — `MCHARBOR_PIN` added to the variable table, with the
  paragraph explaining why it is preferred over `MCHARBOR_INSECURE`.

## 2026-09-21 22:40 — McHarbor 2026.10.01 — The local CA, and installing its root everywhere

### Added
- `screens/security/install-root-ca.md` — a new page for the half of the certificate story the
  set did not cover: getting the root **trusted**. How to obtain the file (download, `docker cp`,
  or pulled out of the served chain with `openssl s_client`), how to verify its fingerprint
  before installing a root that can vouch for any name, which of a machine's two or three
  certificate stores each client actually reads, then the procedure for macOS, Windows,
  Debian/Ubuntu (the `.crt` rename that silently decides whether it worked), RHEL/Fedora, Alpine,
  Chrome and Edge on Linux through NSS, Firefox per profile and by `policies.json`, Android
  (browsers yes, apps no, and why), iOS and iPadOS (install the profile, *then* enable it under
  Certificate Trust Settings), and the non-browser clients — curl, wget, git, Node, Python, Java,
  containers, the agent. Closes with the three warnings that survive a correct install, the SAN
  caveat behind the "name does not match" one, and removal per platform.
- `api/settings/README.md` — `GET /api/settings/tls/ca` in the endpoint table and in the TLS
  prose: what it returns, that only the public half is ever read, and the `404` on an install
  that has only ever served an operator upload.
- `api/settings/reference.md` — the `hasCa` / `caFingerprint` / `caNotAfter` fields the TLS
  status payload carries, noted as read from disk rather than from a settings key.

### Changed
- `getting-started/installation/README.md` — the **TLS** section rewritten around the CA. It
  described a bare self-signed certificate and offered "accept the warning" as the way out;
  it now has the two-file table with the ten-year root and the 90-day leaf, says the SAN list is
  fixed at issue time and that **Regenerate** keeps the root, and makes installing the root the
  first of the three ways out rather than an afterthought.
- `getting-started/first-run/README.md` — boot step 5 names the root and the leaf; "Deal with
  the certificate" is now a procedure (download, install once per device, trust survives
  rotation) instead of "browsers will complain".
- `getting-started/prerequisites/README.md`, `getting-started/configuration/README.md`,
  `getting-started/installation/troubleshooting.md`,
  `getting-started/first-run/troubleshooting.md`,
  `agents/installation/troubleshooting.md` — the remaining "self-signed certificate, accept it"
  wording replaced with the local-root model and a pointer to the install page. The agent's
  `x509: certificate signed by unknown authority` row now offers the root and `MCHARBOR_PIN`
  before `MCHARBOR_INSECURE`, which accepts any certificate at all.
- `getting-started/configuration/README.md` — "Trusting a private or self-signed CA" retitled
  **Trusting a private CA on the way out**, with a line separating it from the root clients
  install to trust McHarbor. The two were one search term apart and mean opposite directions.
- `screens/security/https.md` — the four-row install table became eight, covering the stores
  that browser choice rather than OS choice decides, and defers the detail to the new page.
- `screens/security/README.md`, `screens/security/troubleshooting.md` — the new page indexed,
  plus three symptom rows: a root installed into the wrong store, a warning that names the
  address rather than the issuer, and a phone that cannot sign in to fetch the download.

## 2026-09-21 22:10 — McHarbor 2026.10.01 — Preferred host distribution

### Added
- `getting-started/prerequisites/README.md` — an **Ubuntu Server 24.04.5 LTS or 26.04.1 LTS**
  row at the top of the host table, and a paragraph under it saying what "preferred" means:
  McHarbor is a container and runs anywhere Docker Engine does, but those two releases are the
  combination each release is verified on and the one the documentation's package commands
  assume.
- `agents/prerequisites/README.md` — the same row on the agent host table, with the reason
  that matters for an agent: a host that will run guests has to clear the compute probe, and a
  stock install of either release clears cgroup v2, user namespaces and idmapped mounts with no
  boot parameter changed.
- `plugins/addons/compute-foundation/host-requirements.md` — a **Preferred distribution**
  paragraph closing "Decide this first", naming the four probe items a stock Ubuntu Server
  install already clears and the two that are left to arrange.

### Changed
- `plugins/addons/compute-foundation/supported-platforms.md` — both recommendations now name
  the distribution instead of gesturing at one. "A Linux host on `x86_64`" became Ubuntu Server
  24.04.5 LTS or 26.04.1 LTS for McHarbor itself, and "a current systemd distribution with
  cgroup v2" became the same two releases for the recommended guest host.

## 2026-09-21 21:35 — McHarbor 2026.10.01 — A task index on the agents landing page

### Added
- `agents/README.md` — a **Common tasks on the host** table between the folder index and the
  one-page overview. The host-side tasks an operator performs over SSH — read the log,
  restart, change a variable, raise the log level, rotate, update, uninstall — each point at
  the section that covers them, with the closing reminder that the central settings need no
  SSH. The sections live with their subsystem rather than in `how-to/`, which is scoped to
  extending McHarbor; this table is the findability answer instead.

## 2026-09-21 21:20 — McHarbor 2026.10.01 — Changing a native agent's configuration from the terminal

### Added
- `agents/configuration/README.md` — a **Changing a variable on the host, from the terminal**
  section. The page listed every variable and said in one line that they are read at start-up;
  it never said how to change one. Now: that there is no reload signal, the `systemctl edit`
  drop-in and why it beats editing the installer's unit (a reinstall overwrites that file),
  the two drop-in traps (last assignment wins, and a variable is cleared by assigning it empty
  rather than omitting it), direct unit editing with the `daemon-reload` people skip,
  `EnvironmentFile=` for keeping the token out of the unit, kill-and-relaunch with no
  supervisor, container recreation and why it needs the data volume, `docker inspect` to
  recover a lost run line, how to confirm the change took effect, and which settings are
  central rather than agent-side.

### Changed
- `agents/operations/README.md` — the restart subsection now points at that section for
  configuration changes.

## 2026-09-21 21:00 — McHarbor 2026.10.01 — Reading and restarting a native agent from the host

### Added
- `agents/operations/README.md` — the **On the host** diagnostics subsection grew from two
  lines into the full account for a native install: the systemd journal (follow, tail,
  `--since`, `-b`, priority, `systemctl status`, and the two reasons it looks empty — root-only
  access and volatile storage), the no-systemd `/var/log/mcharbor-agent.log` path and why it
  needs `copytruncate` rotation, macOS and Windows where McHarbor ships no service installer,
  what the `slog` text lines look like and which three a healthy start prints, raising
  `LOG_LEVEL` through a `systemctl edit` drop-in rather than the installer's unit, and the
  offline buffer at `$MCHARBOR_DATA_DIR/offline.db`.
- `agents/operations/README.md` — a **Restarting a native agent** subsection: that a restart is
  safe and reconnects on its own, `systemctl restart` / `status`, `daemon-reload` before
  restarting after a unit edit, kill-and-relaunch on a host with no supervisor, and the two
  cases a restart alone does not fix — a regenerated token and a hand-swapped binary.

## 2026-09-20 22:45 — McHarbor 2026.10.01 — The page-head band, and who owns the gutter

### Added
- `screens/README.md` — a **The page head** subsection in the application frame. The band was
  introduced this afternoon and the frame page never described it: what it holds, that it is
  full width under the top strip, and that it supplies the top spacing so the page area adds
  only the horizontal and bottom gutter. That doubling is what made a screen look like it had
  an extra empty band under its title.

## 2026-09-20 22:30 — McHarbor 2026.10.01 — The dashboard page head counts the estate

### Changed
- `screens/dashboard/README.md` — new **The page head** section. The band under the title is
  now a live estate summary, and the page says what governs it: it is fleet-wide rather than
  scoped to the environment switcher, a count the reader may not see is **absent rather than
  zero**, exception counts (agents behind, scans to review, unreachable hosts) appear only when
  they are non-zero, an unreachable host is reported rather than silently dropped, and the
  virtual-machine segment exists only with the plugin installed and `vm.view` held. It also
  gives "needs review" its definition, since a scan carries no review state: an image whose
  latest completed scan found something at critical or high.
- `api/endpoints/monitoring.md` — `GET /api/dashboard/summary`, with the omitted-not-zeroed
  rule stated on the row.
- `api/endpoints/security.md` — `/api/scans/summary` now also returns `needsReview`.

### Note
- No new permission key. Every figure in the summary keeps the gate it already had, and the
  endpoint computes each one only over the environments the caller holds `environments.view`
  on.

## 2026-09-20 21:30 — McHarbor 2026.10.01 — Two pages in the wrong section, and five index orders

### Changed
- `screens/secrets/README.md` → **`screens/security/secrets.md`**. `/secrets` is a redirect to
  `/security?tab=secrets`, and `secrets` is a tab of the Security screen in a group of its own.
  The page already opened by saying "Reach it from Security → Secrets"; only its location
  disagreed. It was also the one screen folder in the set with a single page rather than four.
- `how-to/guest-networking/README.md` →
  **`plugins/addons/compute-foundation/guest-networking.md`**. The How-to section describes
  itself as guides for *extending* McHarbor — "how do I add one" — and guest networking adds
  nothing; it configures Compute on a host. It was the only guide absent from that page's
  "Which surface do I need?" table, and nothing in the Compute documentation linked to it.
- `screens/security/README.md` — the section table was missing two of the screen's twelve
  sections: `elevations` in the Management group, and the whole `secrets` group. "Ten sections
  in five groups" is now twelve in six, the secrets permission family joins the permission
  table, and `secrets.md` heads the folder table.
- `screens/README.md` — `/docker` was listed between `/networks` and `/logs`, reading as a
  member of the Docker sidebar group. That group is Containers, Stacks, Images, Volumes and
  Networks; the Docker screen has **no sidebar entry at all**. It now sits at the end of the
  map saying so. The Secrets row names the redirect, and the "four pages per screen" rule
  states its exception.
- `plugins/addons/compute-foundation/README.md` — the page table is in reading order:
  Supported hosts and guests → Host requirements → Getting started → Configuration → Guest
  networking → Image catalog → Troubleshooting → Developer. It used to lead with Getting
  started and bury Configuration between two extras.
- `agents/README.md` — **Use cases** moved to first. It is the page that decides whether you
  want an agent, and it sat fourth, after Installation and Configuration.
- `getting-started/README.md` — **Changelog** lifted out of the step list, which is introduced
  as "each step is a folder" and which it is not, into its own reference row, with the note to
  read it *before* Upgrading and that it is the **product** changelog — this file is the
  **documentation** one.
- `plugins/README.md` — the family table ran 6, 18, 11, 14, 5, 3, 2, 2. Now Feature plugins
  first, then by size.
- `screens/environments/README.md`, `plugins/addons/compute-foundation/supported-platforms.md`
  — links follow the two moves.

### Note
- 1957 relative links across the set were checked after the moves; none are broken. The eight
  plugin-family counts were re-checked against the directories and are correct.

## 2026-09-20 21:00 — McHarbor 2026.10.01 — Supported hosts and guests, and the sweep since 18:30 on the 18th

### Added
- `plugins/addons/compute-foundation/supported-platforms.md` — a new page answering three
  questions the documentation had spread across four others and never stated plainly: what
  **McHarbor** runs on, what the **agent** runs on, and what a **guest** may be. Carries the
  agent's five published platforms with what each one can and cannot host, the container
  versus native line, a recommended McHarbor host and a recommended guest host, the guest-type
  matrix, the image catalog broken down by kind (24 ISO, 22 cloud image, 3 root filesystem)
  and architecture (40 `x86_64`, 9 `aarch64`), the 26 distributions it covers, and the guests
  with a precondition worth knowing before you start — Windows 11 and its vTPM, virtio during
  setup, Home Assistant OS decompressing after 100%, cloud-init. Linked from the plugin's page
  list, from `host-requirements.md`, and from both prerequisites pages.

### Changed
- `screens/environments/README.md` — a new environment is **Socket** or **Agent**; the legacy
  TCP, TLS, SSH and Podman types are read and offered only on an environment that already uses
  one. New **Creating one, and getting the agent onto the host** section: Manual versus
  Automatic (SSH), SSH credentials as `${secret:NAME}` references, `su` versus `sudo`
  detection, the two install URLs the result always carries, the temporarily hidden
  Prerequisites panel, and that environments are editable after creation. The detail-tab table
  gains `updates` under Host, with what that tab does and the note that it works over the
  agent now; the agent log card's actions are named; the table row carries the host's own IP.
- `agents/installation/README.md` — the binary endpoint answers `HEAD`, which is what both
  install paths probe with; the generated script retries, resumes and verifies the downloaded
  length against `Content-Length`, and a by-hand `curl` does neither; `MCHARBOR_DATA_DIR` is
  in the documented unit, with what is lost without it.
- `agents/installation/troubleshooting.md` — three rows: a TLS `bad record mac` part-way
  through the download and what it means about the path, a length mismatch on a truncated
  binary, and an unquoted install URL printing the script instead of running it.
- `agents/prerequisites/README.md` — the published platform list corrected to what McHarbor
  actually serves, and a **Which hosts can do what** section separating Docker management from
  guest hosting.
- `getting-started/prerequisites/README.md` — a pointer to the stricter answer for a host that
  will run guests.
- `screens/containers/README.md` — the Data group's tabs read Backup, Schedules and Backup
  list; tabs no longer drag to reorder, and pinning is unaffected.
- `plugins/addons/virtual-machines/README.md` — snapshots as a branching tree with names that
  take spaces, memory snapshots, a revert that keeps the branch below it and offers to stop and
  restart the guest, and in-row progress; the prebuilt guest-agent disc; a console that is
  graphical only and survives navigation; the Logs tab on the container log toolchain; the
  list and card carrying the detail page's actions; one directory per guest with relative
  backing references; and imports rewritten — staged on the host, with progress, cancel,
  restart recovery, a Conversion tab, and Secure Boot and vTPM on the convert form. The old
  claim that a conversion dies with the browser tab is gone, because it no longer does.
- `plugins/addons/compute-foundation/README.md` — `compute-images` and `compute-networks` in
  the widget list, and the new page in the folder table.
- `plugins/addons/compute-foundation/host-requirements.md` — a pointer to the inventory page
  beside this checklist.

### Note
- `CHANGELOG.md`, `README.md` and the What's New overlay were brought up to the same point in
  the same change. They are release notes rather than documentation, so they are recorded here
  only as context for why these pages moved.

## 2026-09-20 20:40 — McHarbor 2026.10.01 — Creating a Compute guest network

### Changed
- `how-to/guest-networking/README.md` — new **Creating the network** section. The page
  explained the three modes and how to convert a macvlan host, but never walked the form
  itself. It now covers where it lives (Compute → Networks → Add network), the permission
  split (`compute.manage` to create, `compute.delete` to remove), the three fields every
  mode asks for, and a table per mode. Managed mode gets the addressing written out — the
  `10.100.0.0/24` and `10.100.0.1` defaults, the DHCP pool derived from the subnet when the
  range is empty, NAT and its required outbound interface, and a worked plan for a host on
  a `10.35.0.0/16` LAN. Also the external-mode 45-second auto-rollback, the server-side
  refusal table from `validateNetworkRequest`, the "agent cannot see the host's network"
  case, and how a guest actually attaches to a network once it exists.

## 2026-09-20 18:28 — McHarbor 2026.10.01 — Running the agent natively, as a service

### Changed
- `agents/installation/README.md` — **As a binary with systemd** rewritten. It now says where
  the binary comes from (`GET /api/agent/binary/{os}/{arch}`, the GitHub release only as a
  fallback, `scripts/development/build-agent.sh` for a local build), gives the install
  sequence rather than the unit alone (`chmod 600`, the compose directory, `daemon-reload`,
  `enable --now`, `journalctl`), and states the thing that was missing: the install script
  always prefers the container when Docker answers, so a native install on a Docker host comes
  from **Deploy over SSH → system binary** or from doing it by hand. Windows and macOS get no
  service installer, and now say so.
- `agents/installation/troubleshooting.md` — two rows: a binary install whose unit is never
  written on a systemd host, and one that connects but cannot open the Docker socket.
- `screens/environments/README.md` — the agent-token dialog's **Binary** tab is the native
  install and now carries download, foreground command and systemd unit as three blocks.

### Note
- The `Binary` tab change is a UI change shipped with this entry, not documentation catching
  up with an older release.

## 2026-09-20 12:35 — McHarbor 2026.10.01 — Add-on objects reach the timeline, and their audit rows name an actor

### Changed
- `screens/lifecycle/README.md`, `screens/lifecycle/fields.md` — the Logging timeline now
  carries **virtual machines** and **storage pools** beside the Docker subjects, and the
  **Source** column names the plugin that produced a row. A guest is recorded on a genuine
  transition only, because the reconciler re-asserts a running guest's state once a minute.
- `screens/audit/fields.md` — the **User** column is blank on a row emitted for work nobody
  asked for directly, and an add-on's actions now take their severity from the action catalog
  rather than falling through to a heuristic that filed everything as Info.
- `plugins/developing.md` — the host handle table gains `Lifecycle`, and records that `Audit`
  also offers `EmitEvent` for a row that should name the account behind the request.

## 2026-09-20 12:00 — McHarbor 2026.10.01 — Secret insert is Shift + right-click

### Changed
- `screens/secrets/README.md` — the secret picker now opens on **Shift + right-click**; a plain
  right-click is left to the browser, which is the reverse of the previous modifier rule.

## 2026-09-18 19:00 — McHarbor 2026.10.01 — Calendar versioning

### Changed
- Every plugin page's **Host range** and `version` row — the manifests moved to `>=2026.09.01`
  and to the host's own version, so the 116 rows recording `>=2.1.0 <3.0.0` and `1.0.x` were
  both stale and wrong. 126 files.
- `how-to/plugin/manifest.md`, `plugins/developing.md` — the `version` field is not strict
  semver: leading zeros are permitted, which is what makes `2026.09.01` a legal version.
- Today's entries in this file now read `McHarbor 2026.09.01`. **Older stamps keep `3.0.0`**:
  they record the version that was current when the page was written, and rewriting them would
  falsify the history.

### Note
- References to an agent floor — "needs agent 3.0.0+", "before 3.0.0 DHCP did not…" — are
  deliberately left alone. The `ComputeMinVersion` family of constants keeps its old numbers, so
  the prose and the code still agree.

## 2026-09-18 18:51 — McHarbor 2026.10.01 — Virtual machine diagnostics, and what the catalog now holds

### Changed
- `plugins/addons/virtual-machines/README.md` — the Logs tab and its two sources, and why the
  software TPM is worth reading separately from QEMU; power controls beside the console and why
  they are gated on `vm.manage` rather than `vm.console`; and that a machine's address comes from
  the QEMU guest agent inside the guest, so it stays blank until that is installed. Route count
  31 → 32.
- `plugins/addons/compute-foundation/README.md` — the catalog is 49 images across desktop, server
  and ARM64 Linux plus Windows and Android; server entries prefer cloud images and why; logos and
  the operating-system filter; and the rule that an entry with no verifiable checksum carries none
  rather than a weaker one, with MD5 explicitly not accepted.

## 2026-09-18 16:07 — McHarbor 2026.10.01 — The guest import dialog survives a stray click

### Changed
- `plugins/addons/virtual-machines/README.md` — the import dialog is no longer dismissed by an
  outside click or Escape, and the import runs as a background job that survives the dialog
  closing, with a chip on the machine list to reopen it. States the remaining limit: the
  conversion is tied to the browser tab that started it.

## 2026-09-18 15:29 — McHarbor 2026.10.01 — Creating a VM with no image, or on a disk that already exists

### Changed
- `plugins/addons/virtual-machines/README.md` — two new create paths. A machine can be created
  with no image and no installer disc, starting at the firmware prompt; and the Storage step can
  adopt a volume the pool already holds that no machine refers to, instead of allocating a new
  one. Notes that neither path runs cloud-init, that nothing is written to an adopted disk, and
  that the free-volume list needs `vm.manage` and an agent that answers `vm_disk_list`. Route
  count 30 → 31.

## 2026-09-18 12:20 — McHarbor 2026.10.01 — Guest networking, corrected against a real conversion

### Changed
- `how-to/guest-networking/README.md` — reattaching containers is split into the two cases
  it actually has: a container whose primary network mode was the deleted network cannot be
  reconnected and must be recreated, and restarting one leaves it running with no network.
  Adds the pre-flight that identifies those containers while the network still exists, the
  `ip -d link show | grep -c macvlan` gate before the netplan change, and a correction to
  the rescue-path advice — a second interface can drop during `netplan apply` too.

## 2026-09-18 11:50 — McHarbor 2026.10.01 — Guest networking

### Added
- `how-to/guest-networking/README.md` — the three compute network modes and their Docker
  analogues; why a VM cannot join a Docker macvlan network; the ordered procedure for
  converting a NIC that already carries macvlan into a bridge, including the netplan
  pitfalls and the scheduled-revert pattern; where a guest's IP address comes from.

### Changed
- `how-to/README.md` — lists the new guide.

## 2026-09-17 01:10 — McHarbor 3.0.0 — The `application` scope

### Added
- `screens/secrets/README.md` — `application` joins the Scope table as a fourth scope: what
  it is for, that it needs `secrets.manage_global`, that it resolves **last** and why that
  ordering is what makes it safe to introduce, and that it is not the same thing as the
  `system = 1` "Application credentials" further down the page.

### Changed
- `screens/secrets/README.md` — resolution order is now stack → environment → global →
  application; the insert picker's tree gained a scope level above kind.

## 2026-09-17 00:20 — McHarbor 3.0.0 — The insert picker is a tree

### Changed
- `screens/secrets/README.md` — "Right-click any field": the picker now lists secrets as a
  collapsible kind → group → secret tree, with Ungrouped last within its kind and the whole
  tree force-expanded while a search query is active.

## 2026-09-16 23:05 — McHarbor 3.0.0 — Secret kinds are open, and the vault list groups

### Changed
- `screens/secrets/README.md` — Groups section rewritten for the creatable group selector,
  the **Edit details** dialog (group + description only) that replaces the folder button,
  and the **Group** switch that draws the table as collapsible sections. Notes that
  Description is no longer a column.

### Added
- `screens/secrets/README.md` — **Kinds** section: the eight built-ins are defaults rather
  than a closed set, custom kinds are created from the same selector, and the shape a
  custom kind has to match.

## 2026-09-16 22:45 — McHarbor 3.0.0 — Secret groups

### Changed
- `screens/secrets/README.md` — new **Groups** section under "Using a secret": the optional
  free-text group label, the Group column, the filter chips, **Move to group**, and the
  point that a group is filing only — it is not part of a secret's identity and never
  changes what `${secret:NAME}` resolves to. Routes table notes the `?group=` list filter
  and that `PATCH` now carries the group.

## 2026-09-16 08:19 — McHarbor 3.0.0 — The image catalog, and sharing per host

### Added
- `plugins/addons/compute-foundation/image-catalog.md` — the downloadable image list is now a
  directory of JSON files bind-mounted at `/mnt/compute-catalog`, one per entry. Every field,
  why a checksum is effectively mandatory, what the reload does to entries you delete, and what
  is rejected outright. Written because a plugin's migrations run once at install, so the SQL
  seed could never be updated on a running instance.

### Changed
- `plugins/addons/compute-foundation/configuration.md` — a new "The downloadable image list"
  section covering the catalog and the per-host shared-library switch, plus the two new routes
  (`images/catalog/reload`, `images/{id}/fetch-all`) in the route table.
- `plugins/addons/compute-foundation/README.md` — links the new page.

---

## 2026-09-16 05:12 — McHarbor 3.0.0 — VM hardware and installer discs

### Changed
- `plugins/addons/virtual-machines/configuration.md` — the route table gains
  `POST /api/vms/{id}/media` and `DELETE /api/vms/{id}/media/{diskId}`, both gated on
  `vm.manage`.
- `plugins/addons/virtual-machines/README.md` — route count 28 → 30, and the behaviour the
  Hardware tab now has: disks, discs, interfaces and host devices are changed there with the
  machine stopped, a disc boots after the disks, and ejecting never deletes the image.

---

## 2026-09-15 13:40 — McHarbor 3.0.0 — Getting started with guests

### Added
- `plugins/addons/compute-foundation/getting-started.md` — nothing to a guest you can log
  into, in seven steps: the native-agent decision, the plugins and their permission keys, the
  four wizard steps, getting an image, creating the guest, the console, and what the
  addresses page does and does not show. Ends with the symptoms this session actually
  produced and what each one means.

### Changed
- `plugins/addons/compute-foundation/README.md`, `.../virtual-machines/README.md`,
  `.../system-containers/README.md`, `.../host-requirements.md` — all four now point at it.

---

## 2026-09-15 13:05 — McHarbor 3.0.0 — What a host needs to run guests

### Added
- `plugins/addons/compute-foundation/host-requirements.md` — the authoritative page: every
  check the capability probe performs with what it blocks and how to fix it, the exact
  `CanRunVMs` / `CanRunContainers` formulas, nested-virtualization cases, and the
  container-versus-native-agent decision stated first.

### Changed
- `plugins/addons/virtual-machines/README.md` and `plugins/addons/system-containers/README.md`
  — a **Before you install: what the host needs** section at the top of each. Neither page
  previously said anything about host requirements, which is where a customer looks first.
- `agents/prerequisites/README.md` — the compute table completed against the probe
  (`qemu-img`, user namespaces and subordinate ids were all missing, and all three are
  critical), now says which capability each item blocks, and links to the new page.

---

## 2026-09-15 06:45 — McHarbor 3.0.0 — Agent container installs on the host network

### Changed
- `agents/installation/README.md` — "The container runs on the host's network" replaces the
  opt-in `--compute` section: a container install now uses `--network host --cap-add
  NET_ADMIN` and publishes no port, with `--isolated` (`MCHARBOR_AGENT_ISOLATED=1`) as the
  opt-out. The by-hand `docker run` and the Compose profile match.
- `agents/troubleshooting/README.md` — the empty-picker row now names `--isolated` or a
  pre-change install as the cause.

---

## 2026-09-15 06:30 — McHarbor 3.0.0 — Managed networking and the external switch

### Added
- `agents/troubleshooting/README.md` — three rows under **Compute hosting**: guests with no
  address on a pre-3.0.0 agent, guests with an address but no route out, and an external
  switch that reverted itself.

---

## 2026-09-15 05:50 — McHarbor 3.0.0 — Compute host network visibility

### Added
- `agents/troubleshooting/README.md` — empty bridge and uplink pickers under **Compute hosting**: a containerised agent sees only its own network namespace, and the new **Host network access** capability item names it.
- `agents/installation/README.md` — the install script's opt-in `--compute` flag. Superseded the same day by the 06:45 entry, which made host networking the default.

---

## 2026-09-15 01:46 — McHarbor 3.0.0 — Uploading your own compute image

### Added
- `plugins/addons/compute-foundation/configuration.md` — the shared image library:
  where uploads are written, the bind every host needs, the two-pass SHA-256, what
  *the image is not in the shared library* means, and why uploading has its own
  permission key.

### Changed
- `plugins/addons/compute-foundation/README.md` — `compute.images.upload` added to the
  permissions table, route and migration counts refreshed, and a pointer to the library
  section.

---

## 2026-09-14 18:50 — McHarbor 3.0.0 — Self-signed toggle in the agent install dialog

### Changed
- `screens/environments/README.md` — the install dialog's self-signed-certificate toggle,
  what it changes in each of the four methods, and why it defaults to on.

---

## 2026-09-14 18:30 — McHarbor 3.0.0 — Compose in the agent install dialog

### Changed
- `screens/environments/README.md` — the install dialog now offers four methods in tabs
  (script, Compose, `docker run`, binary), and says why the Compose tab is the one to
  prefer for a host you intend to keep.

---

## 2026-09-14 18:10 — McHarbor 3.0.0 — Migrating to an agent installs the agent

### Added
- `screens/environments/actions.md` — **When a migration fails or stalls**: the three
  failure paths, what each reverts, and the 15-minute deadline after which an agent that
  never connected is rolled back and its container removed.

### Changed
- `screens/environments/actions.md` — migrating to an agent now deploys the agent container
  itself through the connection the environment already has; documents the
  **Install the agent for me** and self-signed-certificate switches, and that the old
  endpoint is kept in reserve for the rollback.

---

## 2026-09-14 17:45 — McHarbor 3.0.0 — Environment connection migration and merge

### Added
- `screens/environments/actions.md` — **Migrating a connection**: what the in-place
  socket ↔ agent conversion keeps, why the cutover waits for the agent to authenticate,
  and what leaving an agent revokes. **Merging duplicate environments**: the dry-run plan,
  the schema walk behind it, and the two rules that decide collisions and secrets.
- `screens/environments/README.md` — **One host, one environment**: why a socket and an
  agent environment for the same machine is a reporting and polling problem, and that
  McHarbor detects the pair by Docker daemon id.

### Changed
- `screens/environments/actions.md` — action table carries **Migrate connection** and
  **Merge duplicate**.

---

## 2026-09-14 17:05 — McHarbor 3.0.0 — A data location instead of a storage pool

### Added
- `plugins/addons/compute-foundation/configuration.md` — **Pools without a pools page**:
  the `POST /api/compute/pools/default` route, and why the directory backend is offered
  on every host even when `qemu-img` is missing.
- `plugins/addons/virtual-machines/configuration.md` — **Storage**: the create wizard asks
  for a data location when the host has no pool, and provisions the directory pool itself.

### Changed
- `plugins/addons/compute-foundation/configuration.md` — route table carries the new
  ensure-default-pool route.

---

## 2026-09-14 13:35 — McHarbor 3.0.0 — Apply continues, stale references pruned

### Added
- `screens/secrets/README.md` — **References to things that no longer exist**: what the
  missing-resource probe does, why a prune is a deliberate action rather than a silent
  side effect of opening the dialog, and why the probe is concurrent and bounded.

### Changed
- `screens/secrets/README.md` — *Apply* no longer stops at the first failure. It runs every
  selected resource in turn and reports each one separately; the guidance now says that a
  run where everything failed points at the rotated value rather than the resources.
- `screens/secrets/README.md` — the route list gains
  `POST /api/secrets/{id}/references/prune`.

---

## 2026-09-14 11:45 — McHarbor 3.0.0 — Rotation impact and apply

### Added
- `screens/secrets/README.md` — a rewritten **Rotating** section covering what a rotation
  actually reaches. Module credentials and workflows are live; containers and stacks are
  not, and the new version stamp on every reference is what makes "out of date" an
  answerable question rather than a guess.
- `screens/secrets/README.md` — the impact dialog, the `secrets.apply` permission, the
  sequential stop-at-first-failure apply, the in-place file-mode refresh and its
  `com.mcharbor.secret-signal` label, and the per-secret automatic apply.

### Changed
- `screens/secrets/README.md` — the permission table gains `secrets.apply`, and the route
  list gains `GET /api/secrets/{id}/impact` and `POST /api/secrets/{id}/apply`.

---

## 2026-09-14 09:10 — McHarbor 3.0.0 — Log severity colouring

### Changed
- `screens/logs/README.md` — records that log lines are now tinted by the severity word
  they contain, and that the level is read out of the line text rather than supplied by
  Docker, so a format that never names its level stays uncoloured.
- `screens/logs/fields.md` — the **Log line** column description names the tint.

---

## 2026-09-13 16:20 — McHarbor 3.0.0 — Release notes brought up to date

### Added
- `settings/system/database/reference.md` — **What each backend is verified against**. The
  116-file migration set is applied from empty against SQLite, PostgreSQL 16, MySQL 8 and
  MariaDB 10.6; the files are never rewritten and SQLite receives them byte for byte. Names
  the three constructs skipped on the external drivers — the audit-log WORM triggers, one
  expression index on MariaDB, and `068_rbac.sql`'s conditional key renames — with what each
  costs, and the one semantic difference (`ON DELETE SET NULL` becoming `NO ACTION` on
  MySQL).

### Changed
- `getting-started/changelog/README.md` — the 3.0.0 section and its release-history row are
  re-dated to 2026-09-13 and list what landed after the first draft of the release: the
  secrets vault, the three external database backends, licensing, per-container Recreate,
  custom branding, agent health and uptime, log search, and the assistant's authoring
  tools. Plugin count 46 → 61.

## 2026-09-13 16:40 - McHarbor 3.0.0 - The secret mount is gated, and mounts are audited

### Added
- `screens/secrets/README.md` - **The container Files tab needs `secrets.reveal`**. The
  `/run/secrets` mount is hidden from the quick-access cards and filtered out of a listing
  of `/run` for anyone without the permission; reads and writes under it return 403 and are
  audited at warning severity, permitted or not.
- `screens/secrets/README.md` - **Every mount and volume is audited in the Files tab**. What
  `file_list`, `file_read`, `file_download` and the six mutation actions record, the
  `mount=` / `volume=` / `type=` fields they carry, innermost-mount-wins attribution, and
  why reads outside a mount are deliberately not logged.

### Changed
- `screens/secrets/README.md` - the `secrets.reveal` row in *Permissions* now says it also
  gates browsing the mount.

## 2026-09-13 15:31 - McHarbor 3.0.0 - AI assistant can author automation

### Changed
- `api/endpoints/ai.md` - **The assistant can author automation, and only what you could
  author yourself**. New paragraph under *Things worth knowing first* covering the four
  consent-gated create tools (workflow, alert rule, update policy, schedule), the
  `ai_assistant.act` + `<module>.manage` pair each requires, the re-check at execution
  time, workflows landing as drafts, and the read-only `alerts.conditions` lookup the
  assistant uses to build a valid rule. No endpoint changed - the tools run through the
  existing consent and execute routes.

## 2026-09-14 04:25 - McHarbor 3.0.0 - File mode on an existing container

### Added
- `screens/secrets/README.md` - **Changing it on a container that already exists**. The
  Recreate confirm dialog now carries the file-mode switch, pre-set to how the container
  was built, because Docker fixes env and mounts at creation and there is no live toggle.
  Documents the `com.mcharbor.secret-refs` label that makes it possible - a running
  container's environment holds only the resolved value, so the references (names, never
  values) are kept on the container and restored before re-planning - and why the switch
  is offered only where it would change something.

### Changed
- `screens/secrets/README.md` - the "where the toggle appears" table gains the container
  detail Recreate dialog as a fourth entry.

---

## 2026-09-14 03:10 - McHarbor 3.0.0 - File-based secret injection

### Added
- `screens/secrets/README.md` - a **File mode** section. Opt in per container, per App
  Store install or per stack, and a referenced secret is written into a read-only volume
  at `/run/secrets` with `<VAR>_FILE` pointing at it, instead of the plaintext going into
  the container's environment where `docker inspect` shows it. Documents the three places
  the toggle appears, the generated `docker-compose.mcharbor-secrets.yml` (your own
  compose file is never edited), the `_FILE` image-support requirement, and the two real
  limits - only a whole-value reference converts, and it is no boundary against anyone
  holding the Docker socket.

### Changed
- `screens/secrets/README.md` - "cannot hide a value from `docker inspect`" is now
  qualified: that is the default, not the ceiling.

---

## 2026-09-14 01:45 - McHarbor 3.0.0 - Secrets moved to Security, right-click insert

### Changed
- `screens/secrets/README.md` - the vault is now **Security -> Secrets**, not Docker ->
  Secrets. It holds registry passwords, git credentials, SMTP settings, TLS certificates,
  OIDC client secrets and agent enrolment tokens; stack environment variables were only
  the first thing it served. `/secrets` still redirects there.
- `screens/README.md` - the screen index follows the move.

### Added
- `screens/secrets/README.md` - **Right-click any field**: right-clicking a text or number
  input anywhere in McHarbor opens a picker of stored secrets and writes the chosen
  `${secret:NAME}` reference into the field. Names and scopes only - the picker cannot read
  a value. Shift+right-click gives the browser menu instead, and the picker never appears
  without `secrets.view`.
---

## 2026-09-13 23:40 - McHarbor 3.0.0 - Secrets vault

### Added
- `screens/secrets/README.md` - the new **Docker -> Secrets** page: what the vault protects
  against and what it honestly cannot (a `docker inspect` on a running container still
  shows the value); `${secret:NAME}` references and the `$${...}` escape; stack ->
  environment -> global scope precedence; the five `secrets.*` permission keys; the reveal
  path (an approved elevation **plus** step-up re-authentication **plus** a recorded
  reason, rate limited to ten an hour, audited before the value is returned and notified to
  administrators); rotation, rollback and the redeploy banner; application credentials and
  the two categories deliberately excluded from the vault (MFA material, machine-rotated
  OAuth tokens); and the **`ENCRYPTION_KEY` backup obligation**, which is now critical
  rather than merely advisable - the vault holds every module's credentials after the
  Phase 5b unification.

### Changed
- `screens/README.md` - Secrets added to the screen index.
---

## 2026-09-13 11:05 — McHarbor 3.0.0 — Per-container Recreate

### Changed
- `screens/containers/actions.md` — added **Recreate** to the action table: it replaces a
  container from its current image and configuration, and is now reachable per container
  from the table row, the card and the detail hero, not only as the bulk "Recreate All".

## 2026-09-13 02:37 — McHarbor 3.0.0 — Plugin-declared OAuth defaults (zero-config sign-in)

### Added
- `plugins/developing.md` — **Zero-config OAuth sign-in (AI providers)**: how an AI adapter
  declares `AIAdapterSpec.OAuthDefaults` (client ID + endpoints, never a secret) so the
  operator signs in with one click, plus the rules that keep it legitimate (no credential-
  store reads, no client impersonation, operator override wins).

### Changed
- `api/endpoints/ai.md` — documented the `/api/ai-providers/oauth-defaults` and
  `/api/ai-providers/{id}/oauth/{status,begin,poll,disconnect}` endpoints; noted that
  `oauth-defaults` now merges plugin-declared client IDs and flags complete configs with
  `pluginProvided`; added `authMethod` / `oauth` to the create/update request bodies.

## 2026-09-13 17:45 — McHarbor 3.0.0 — Reports default to the application logo

### Added
- `plugins/addons/reporting/configuration.md` — **Report branding**: the three-layer
  resolution (report settings → application logo → compiled-in mark), why it is the
  light variant, and why an SVG application logo still works despite the exporters
  embedding raster only.

### Changed
- `settings/general/themes/README.md` — the tab is now **Customization**, and the
  application logo leads the page.

---

## 2026-09-13 17:05 — McHarbor 3.0.0 — Per-variant application logo

### Changed
- `settings/general/themes/README.md` — **Application logo** is now four slots (wordmark
  and icon, each light and dark). Adds the resolution table showing that the dark slot is
  an override falling back to the light upload and then to the McHarbor mark, and notes
  that each preview paints its own ground rather than the active theme's.

---

## 2026-09-13 15:40 — McHarbor 3.0.0 — Custom application logo

### Added
- `settings/general/themes/README.md` — **Application logo**: the wordmark/icon split and
  why there are two uploads, the format and size limits, that the login page shows it to
  anyone who can reach the install, and why the preview sits on the sidebar surface.

### Changed
- `settings/general/README.md` — Themes row now names the application logo.
- `settings/general/themes/README.md` — intro and "What it governs" name the logo.

---

## 2026-09-13 00:48 — McHarbor 3.0.0 — Workload heatmap widget, redesigned sign-in

### Added
- `screens/dashboard/README.md` — the `workload-heatmap` widget: one square per
  container, click a square for its details card.

### Changed
- `screens/dashboard/README.md` — widget count 21 → 22.

---

## 2026-09-13 00:14 — McHarbor 3.0.0 — Trusting a private CA

### Added
- `getting-started/configuration/README.md` — **Trusting a private or self-signed CA** under
  Network trust: the new `MCHARBOR_LOCAL_CA_PATH` mount, why an untrusted chain fails
  outright for Go where a browser would offer a click-through, why there is deliberately no
  skip-verification option, and the two details that bite (the `.crt` extension, PEM not
  DER). Written after a licence activation failed with `x509: certificate signed by unknown
  authority` against an internal licence server.

## 2026-09-12 23:43 — McHarbor 3.0.0 — Agent health and uptime

### Changed
- `agents/operations/README.md` — new **Health and uptime** section. An agent now has a
  derived health (`healthy` / `degraded` / `unhealthy` / `unknown`) alongside its
  connected/disconnected status, derived from the keepalive rather than from the socket;
  the table gives each state, the 2000 ms round-trip threshold and why that figure, and
  the two new Overview cards (health strip plus round-trip chart, uptime strip plus outage
  count). Also records the 30-second sampling cadence, the 7-day retention, that collection
  costs no agent traffic, and that only the HA leader samples.
- `agents/operations/README.md` — `GET /api/agents/{id}/health/history` added to the API
  surface table, gated by `environments.view`.

## 2026-09-12 15:52 — McHarbor 3.0.0 — Release history in Getting started

### Added
- `getting-started/changelog/README.md` — the operator's view of the release notes: the
  versioning scheme and what major/minor/patch each mean here, the remote agent's separate
  version line and its feature floors, the 3.0.0 release with its seven breaking changes as
  a table, and a release history from 3.0.0 back to 1.1.0. The root `CHANGELOG.md` stays
  authoritative and is linked from the page rather than copied into it — it is 10,343 lines,
  and a second copy would drift within a release.
- `getting-started/changelog/README.md` — a note that the root file repeats several
  releases below the 1.1.0 entry (`2.0.0`, `2.0.2`, `1.6.5`, `1.6.6`, and an `[Unreleased]`
  block), and that the first occurrence reading from the top is the current one. Readers
  who scroll the raw file hit the duplicates and have no way to tell which is live.

### Changed
- `getting-started/README.md` — Changelog added to the step table and to "Where to go next".
- `getting-started/upgrading/README.md` — "Read the release notes" now links to the new page
  and says to read **every** version being skipped, not only the target; the page is added
  to Related.

## 2026-09-12 15:13 — McHarbor 3.0.0 — Plugin dependencies and storage capacity

### Added
- `how-to/plugin/manifest.md` — a **Dependencies** section for the `requires` manifest
  field: what the installer pulls in, what uninstall removes (only what it installed
  itself), and the two things it deliberately is not — a version resolver, and a hard
  gate. `requires` added to the top-level field table.
- `how-to/plugin/backend.md` — **Optional contracts**, documenting `StorageQuotaReporter`:
  the `Quota` signature, `plugin.ErrUnsupported` as the capability answer, and the
  `plugin.adapter.storage.quota` audit entry for a real failure.
- `plugins/architecture.md` — dependency install and uninstall behaviour under what the
  host mounts.
- `settings/integrations/storage/README.md` — a **Capacity** section: the bar's amber and
  red thresholds, where the number comes from per destination type, and that a missing
  number means unknown rather than full.
- `settings/integrations/storage/reference.md` — the capacity bar in the settings table,
  and two rows in the symptom table for a missing bar and a failed quota read.
- `api/endpoints/backups.md` — `GET /api/storage-locations/{id}/usage` returns
  `supported: false` with **200**, not an error, when the provider reports no quota.

### Changed
- `how-to/plugin/manifest.md` — the scope-token list now includes `compute.rpc` and
  explains what declaring it grants.
- `plugins/storage/README.md` — every bundled storage plugin except FTP reports capacity.

## 2026-09-11 22:46 — McHarbor 3.0.0 — Unsigned plugins install with a risk warning

### Changed
- `plugins/architecture.md` — "Trust and signing" now describes the shipped policy: an
  unsigned or untrusted tarball installs, is confirmed by the operator in the install
  dialog, and is recorded (`signed: false`, `unsigned: true` on the audit entry, an
  **Unsigned** badge in the App Store). Documents
  `MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true` as the opt-in strict policy, including the
  misconfiguration case where it is set with no trust keys.
- `plugins/overview.md` — install step 2 and the opening definition no longer describe a
  plugin package as necessarily signed.
- `plugins/README.md` — same correction to the opening sentence.

## 2026-09-11 20:25 — McHarbor 3.0.0 — Terminals report colour support

### Changed
- `screens/environments/actions.md` — terminal sessions now advertise
  `TERM=xterm-256color` and `COLORTERM=truecolor`, and the sixteen ANSI colours follow the
  active theme. Notes that `TERM` lets a program choose colour rather than guaranteeing
  it, so a plain `ls` stays plain on an image that does not alias `ls --color`.

---

## 2026-09-11 19:10 — McHarbor 3.0.0 — Fullscreen on every terminal

### Added
- `screens/environments/actions.md` — the terminal control row ends in a fullscreen
  toggle, `Esc` leaves it, and the same button is on the container terminal and the
  sidebar Terminal page. Notes that the shell is resized to match, so the extra rows and
  columns are real rather than a stretched image.

---

## 2026-09-11 18:40 — McHarbor 3.0.0 — Host terminal gains shell, user and working directory

### Added
- `screens/environments/actions.md` — new "Opening a host shell" section: the Console →
  Terminal tab now offers the same shell / user / working-directory controls as the
  container terminal, what each defaults to, that a missing shell falls back rather than
  failing, that the user switch uses `runuser` with an `su` fallback, and that an older
  agent ignores the choices and connects as root to its detected shell.

---

## 2026-09-11 18:10 — McHarbor 3.0.0 — Host logs notices explained

### Changed
- `screens/environments/troubleshooting.md` — new "Reading the Logs tab's notices" section
  distinguishing the grey informational strips (which log files the host exposes, which
  file was read as a fallback) from the yellow warnings, with a table of what each warning
  means. Three new symptom rows: an empty Logs tab on an agent environment needs agent
  1.8.0+ and a connected agent, and on a local Docker Desktop environment the "host" is
  Docker Desktop's own Linux VM, which has neither useful `/var/log` content nor
  `journalctl` — so both the Logs tab and the Host terminal address that VM, not your
  machine.

---

## 2026-09-11 15:52 — McHarbor 3.0.0 — AI provider sign-in as an alternative to an API key

### Changed
- `settings/integrations/ai/README.md` — new "Two ways to authenticate" section: a
  connection carries either a pasted API key or an OAuth 2.0 device-flow sign-in, the
  choice is made on the configuration step and can be changed from Edit, and the sign-in
  uses an OAuth application the operator registers themselves.
- `settings/integrations/ai/reference.md` — the settings table lists the auth-method
  choice and the five sign-in fields; Send test now covers token refresh, and the
  troubleshooting table gains the three sign-in failures.

## 2026-09-11 15:45 — McHarbor 3.0.0 — Detected drivers grouped by origin

### Changed
- `settings/integrations/snapshots/README.md` — the Detected drivers list is grouped by
  origin under a heading for each, rather than printing the origin per row in a flat list.

## 2026-09-11 15:10 — McHarbor 3.0.0 — Backup failure detail, backup alert conditions, certificate SANs

### Changed
- `screens/backups/troubleshooting.md` — a failed run's info button now carries the
  underlying error, not a pointer to the container logs, so the "no obvious reason" row
  points at the dialog first and the log tab second. Notes what is redacted out of it.
- `getting-started/installation/README.md` — the TLS section lists the SANs the
  first-boot certificate actually carries: `host.docker.internal` and `::1` alongside
  localhost and loopback, plus every address bound to a live non-loopback adapter, so the
  certificate is valid from another machine on the network. Adds that the certificate is
  generated once and keeps its SAN list until it is regenerated.
- `getting-started/configuration/README.md` — `SELF_SIGNED_SAN_EXTRA` records that IP
  entries become IP SANs rather than DNS names; `HOST` records that it feeds the SAN list
  only when it names one interface.

## 2026-09-11 13:20 — McHarbor 3.0.0 — Plugin permission gates, and the communication-plugin install fix

### Changed
- `api/conventions/permissions.md` — the three routes the previous entry recorded as
  authenticated-but-ungated are now gated in code, and the page records what gates them and
  why the plugin-loader **reads** stay open (the frontend loader fetches menu, translations,
  widgets and bundles for every signed-in user).
- `api/endpoints/store.md` — the warning section is replaced by the permission table the
  plugin routes now carry; every plugin route row names its key.
- `api/endpoints/host.md` — `POST /api/lifecycle` now shows `settings.manage`; the note
  explains why the two reads stay open.
- `how-to/plugin/manifest.md`, `how-to/permissions/README.md` — document the permission-key
  grammar (`^[a-z][a-z0-9_.]{0,62}[a-z0-9]$`: no hyphens, no colons) and the install-time
  failure it causes, which is what blocked every communication plugin. Example keys corrected
  from `my-plugin.read` to `my_plugin.read` — the original was itself invalid.

### Notes
- Code changes recorded in
  `.references/backup/2026-09-11-1320-plugin-gates-and-comm-install/CHANGES.md`: the gated
  plugin mount wired and the ungated one removed, `plugins.manage` on the runtime reload,
  `settings.manage` on the lifecycle write, the installer's stale scope allow-list replaced by
  the canonical one, the 11 communication-plugin permission keys renamed, and the example
  skeleton's manifest header corrected from `//` to `#`.
- Verified against the real validators: all 62 plugin manifests now pass, where 23 failed
  before.

---

## 2026-09-11 13:02 — McHarbor 3.0.0 — API reference and how-to guides

### Added
- `api/README.md` — the section index: base path, ports, the envelope, a quick start, and how
  to read the reference.
- `api/conventions/` — the response envelope and every field in it, status codes and what each
  actually means here, request and response headers, the body and rate limits, CORS,
  maintenance mode, the middleware order, and the three route groups.
- `api/conventions/authentication.md` — session cookie and its idle-versus-absolute lifetimes,
  API keys and why a key cannot outrank its owner, the two-step MFA login, lockout, first-run
  setup, password reset, SSO, SCIM tokens, agent tokens, the internal transfer endpoints,
  elevation, and what happens when auth is disabled.
- `api/conventions/permissions.md` — the key shape, the five scopes and the six middleware
  variants, where a grant comes from, the full 114-key catalog with scope and description, the
  seven catalog keys that currently gate no route, and every mutating route that carries no
  permission middleware with the reason for each.
- `api/conventions/parameters.md` — `?env=` and why omitting it is a latent bug, pagination
  and the silently-ignored oversized page size, the four filter patterns, path parameters,
  boolean handling, destructive confirmations, optional-field semantics, and `?vacuum=`.
- `api/conventions/errors.md` — how to read a code, the codes that appear everywhere, the ones
  worth handling specifically, and all 660 codes grouped as the source groups them.
- `api/conventions/streaming.md` — the twelve SSE endpoints and the frames they emit, the four
  WebSocket endpoints, console ticketing, the agent transport's two easy-to-regress
  constraints, and the long-running work that is polled rather than streamed.
- `api/endpoints/` — 21 pages covering all **516 mounted endpoints**, one per area, each with
  the permission keys that govern it, the non-obvious behaviour, a full route table (method,
  path, permission, description, parameters) and the request-body fields per endpoint.
- `api/settings/README.md` — the settings API: generic key-value versus typed group
  endpoints, why an out-of-range generic write silently does nothing, the two ungated reads,
  per-user overrides, secrets, and the backup-key and TLS flows.
- `api/settings/reference.md` — **every setting key**: category, JSON field, type, default,
  accepted range and effect, across agent, scanner, retention, toast, appearance, audit,
  security, general, disk-usage, self-update, licence and TLS — plus the five scanner keys
  that are no longer read, and the three families that look like settings and are not.
- `how-to/README.md` — the guide index, which extension surface to pick, and the three
  constraints that catch everyone.
- `how-to/plugin/` — build a plugin end to end, plus a page each for the manifest (every
  field and the installer's ten validation steps), the backend (lifecycle, handler shape,
  host handles, migrations, adapters), the frontend (the UMD contract, the four UI hook
  points, why the page renders blank) and packaging (build, sign, catalog, the developer
  loop, uninstall's hard failure).
- `how-to/permissions/README.md` — contracting a permission: naming and scope, the five steps
  for a host key, backend enforcement and its exemptions, the frontend honesty obligation,
  plugin key declaration, and a worked four-key split.
- `how-to/widgets/README.md` — built-in versus plugin widgets, categories, sizes, the
  imperative mount contract, and the context object.
- `how-to/nodes/README.md` — the message contract and the clone-do-not-mutate rule, the three
  ways to add a node, config field types and when `text` is the wrong one, and testing a node
  end to end.
- `how-to/ai/README.md` — the AI adapter spec, capabilities as promises, the three client
  methods and their concurrency requirement, connections versus providers, and the
  use/act/manage consent model.
- `how-to/reporting/README.md` — the shipped reporting add-on read as eight design decisions:
  reading through the host API as the security model, the permission split, menu gating,
  generation as a job, audit metadata in the manifest, contributing alert conditions,
  rendering, and scheduling without escalation.
- `how-to/api/README.md` — scripting the API: credentials, naming the environment, branching
  on `code`, paging, streams, polling, confirmations, and the eight things that bite.

### Changed
- `README.md` — adds the API and How-to sections to the section table, the "start here"
  table and the folder-layout example.
- `CHANGELOG.md` — entries now carry a **date and local 24-hour time**; the format block and
  the note above it say so.

### Notes
- Three findings are documented where they occur rather than smoothed over, because the code
  and its own comments disagree: `/api/appstore/plugins/*` (six mutating routes),
  `POST /api/plugins/loader/reload` and `POST /api/lifecycle` are authenticated but carry no
  permission gate in the running server. A correctly gated mount for the plugin routes exists
  in the module's `routes.go` and nothing wires it.

---

## 2026-09-11 — McHarbor 3.0.0 — Workflows

### Added
- `workflows/README.md` — what a workflow is, the execution model in six sentences, where
  nodes come from, and how workflows differ from schedules and webhooks.
- `workflows/getting-started/` — a worked first workflow, and the five things worth knowing
  early: deep-copied branches, a failed node not failing the run, skip versus disable, the
  2000-step and five-minute caps, and map-only paths.
- `workflows/editor/` — canvas, palette, the four-tab configuration panel, sniffers, groups,
  blocked ports, import and export, and workflow variables.
- `workflows/execution/` — the queue walk, the step and timeout limits, ports and emissions,
  blocked ports enforced at both ends, skip / disable / blocked groups, the two failure paths
  and how try-catch frames resolve, run statuses, and the live event stream.
- `workflows/messages/` — the `msg` contract, deep copies by JSON round trip, dot-notation
  paths and why they do not index arrays, the msg / flow / global scopes, and the reserved
  underscore keys.
- `workflows/triggers/` — the nine trigger types, what Active actually arms, webhook path and
  method matching with constant-time secret comparison, the Schedule Trigger carrying its own
  timezone field, and link-in / link-out composition.
- `workflows/runs/` — the run list, why `completed` is not "everything worked", the four debug
  surfaces, and a procedure for finding where a run went wrong.
- `workflows/nodes/` — the catalog index plus one page per category (triggers, actions, logic,
  utility, integrations) listing every one of the 86 built-in nodes with its key, ports and
  full configuration schema, taken from each node's own definition.
- `workflows/developer/` — an index comparing the three ways to add a node, plus `core-nodes/`
  (the catalog folder and the `ExecuteNode` case, with conventions and a checklist),
  `custom-nodes/` (the goja sandbox, its bindings, the 10s default and 60s maximum timeout,
  the 100-log-line cap, and the fact that there is no host bridge), and `plugin-nodes/` (the
  `plugin:<slug>:<key>` shape, manifest declaration, and what happens on uninstall).
- `workflows/reference/` — every route with its gate, the canvas and run data model, the SSE
  event payloads, the runtime limits, and where each thing is stored.
- `workflows/troubleshooting/` — symptom, cause and fix across triggering, unexpected
  behaviour, failures, missing nodes and performance.

### Changed
- `README.md` — the root index gains the Workflows section.
- `screens/workflows/README.md` — cross-links into the new section.

One finding recorded rather than glossed: `workflows.view` exists in the RBAC catalog, gates
the sidebar entry and is required by an AI assistant tool, but is **not enforced on the
workflow read routes** — list, get, runs, export and the live SSE stream are session-protected
only. The mutating routes are gated.

---

## 2026-09-11 — McHarbor 3.0.0 — Remote agent

### Added
- `agents/README.md` — what the agent is, the six situations it answers, and the
  connection in one page.
- `agents/prerequisites/` — host requirements, the one-direction network rule, TLS trust,
  what the agent can do once it is there, and the extra requirements for compute hosting
  (systemd as PID 1, cgroup v2, /dev/kvm and virtualization extensions, QEMU and OVMF,
  idmapped mounts).
- `agents/installation/` — the install script and what it detects, the container command,
  the systemd unit as generated, SSH deployment, verification, reinstalling and
  uninstalling; plus a troubleshooting page for installs that do not connect.
- `agents/configuration/` — every agent environment variable with its default, the offline
  buffer row caps, and the host-side settings that govern all agents at once.
- `agents/use-cases/` — NAT and firewalled hosts, homelabs, not exposing the Docker socket,
  compute hosting, cross-host moves, disconnected sites, backups over a thin link, and
  diagnosing a broken host.
- `agents/protocol/` — the handshake and backoff, every WebSocket message family, why
  hijacked streams need their own families, the stream-ordering hazard, offline replay and
  self-update.
- `agents/operations/` — the agent API routes and their gates, token lifecycle, version
  sweeps, licence seats, and diagnostics including the agent-to-agent transfer test.
- `agents/troubleshooting/` — symptom, cause and fix across connection, performance,
  terminals, transfers, offline replay, compute and licensing, plus a short procedure for
  working out where a problem lives.

### Changed
- `README.md` — the root index gains the Agents section.
- `settings/system/license/README.md` — corrected. An instance id identifies a process, so
  a server and an agent on one host mint two; the licence server collapses them into one
  seat using a machine fingerprint, which is a hint rather than a credential.

Facts come from `src/agent/` (`main.go`, `protocol.go`, `reconnect.go`,
`offline_store.go`, `update.go`, `machine.go`, `compute/capability/probe_linux.go`),
`src/backend/modules/agent/` (`install.go`, `routes.go`), the shipped Compose `agent`
profile, and `core/rbac/permissions.go`.

---

## 2026-09-11 — McHarbor 3.0.0 — Getting started, screens and settings

### Added
- `README.md` — the root index: the four sections, how the set is organised, and the
  conventions it follows.
- `getting-started/` — index plus a folder per step: `prerequisites/`, `installation/`
  (with its own troubleshooting page), `first-run/` (likewise), `configuration/` (every
  environment variable the host and the agent read, with defaults), and `upgrading/`.
- `screens/` — index carrying the full route map and the application frame (sidebar, the
  disabled-not-hidden convention, the environment switcher, search, tabs, shared list
  behaviour, live data), plus **one folder per screen** with four pages each — `README.md`,
  `actions.md`, `fields.md`, `troubleshooting.md` — for containers, stacks, images, volumes,
  networks, docker, logs, terminal, activity, lifecycle, backups, reconciler, workflows,
  environments, git, gitops, store, audit, security, dashboard, notifications and profile.
- `settings/` — index, six group READMEs, and **one folder per section** with `README.md`
  and `reference.md`: general, themes, notifications, ai, communications, storage,
  snapshots, logging, catalogs, alerts, data-retention, schedules, webhooks,
  updates-containers, updates-agents, updates-mcharbor, database, agent, licence and about.

Facts come from `docker-compose.yml`, `docker/Dockerfile`, `docker/docker-entrypoint.sh`,
`core/config/config.go`, `core/rbac/permissions.go`, `frontend/core/router.tsx`,
`resources/layout/sidebar-data.ts`, the `*-tab-groups.ts` files for containers, settings,
security, docker and environments, each module's `routes.go` for the gates quoted, and the
shipped `en` i18n namespaces for labels, columns, filters and help text.

No screenshots anywhere: interface strings are quoted instead, so the pages stay true
through a restyle.

Two drifts recorded rather than reproduced: the commented-out `AUDIT_LOG_RETENTION_DAYS` in
`docker-compose.yml` is read by nothing in the code, and mounting `/var/run/docker.sock`
with `:ro` does not restrict the Docker API.

Six sections have no dedicated permission key and are covered by `settings.manage`:
Settings → Snapshots, Catalogs and Agent, and Security → HTTPS, Scanning and Password.

---

## 2026-09-11 — McHarbor 3.0.0 — Plugin documentation, one folder per plugin

This file starts here. The previous changelog, along with every other page that had been
under `.documentation/`, was untracked and was removed from disk before this entry was
written; only 21 pages existed in git and those are an older structure. The section below
is therefore the first entry of a new set rather than a continuation.

### Added
- `plugins/README.md` — the section index: the eight families, what is in a plugin's
  folder, and the two conventions (the host owns the admin surface; the manifest is the
  source of truth) that explain the shape of everything else.
- `plugins/overview.md` — what a plugin is, feature plugins versus adapter plugins, the
  nine adapter families with their registries, and the install / boot / uninstall
  lifecycle.
- `plugins/architecture.md` — the runtime internals: the `-buildmode=plugin` process
  model and its three consequences, why `core/plugin` and `core/pluginruntime` are split
  (the `plugin.Open` per-package ABI hash), the SDK package table, on-disk layout, the
  trust-key policy, what the loader mounts, boot order, every loader endpoint, clustering,
  and a load-failure table.
- `plugins/developing.md` — the author's guide: folder layout, the manifest with the rules
  the installer actually applies, the full scope-token table, the `Plugin` interface, the
  `HostContext` handles, `ConfigFieldDecl`, migrations, permissions, the frontend bundle,
  i18n, build and signing, the developer loop, and a debugging table.
- `plugins/catalog.md` — all 61 shipped plugins in one table by family, with the host
  ranges they support.
- `plugins/<category>/README.md` for the eight families — `addons`, `ai-providers`,
  `communication`, `identity`, `logging`, `scanners`, `snapshots`, `storage` — each
  covering what the family has in common and how to choose within it.
- **One folder per plugin**, 61 of them, three folders deep at most
  (`plugins/<category>/<slug>/`), each with four pages: `README.md` (what it is, where it
  appears, the permission that gates it, how it works, the capability flags),
  `configuration.md` (prerequisites, every declared field, setup order, host scopes),
  `troubleshooting.md` (symptom / cause / fix, and what **Test** really does), and
  `developer.md` (the adapter it registers, what the host calls and when, its Go file
  layout, its full manifest, and how to build, sign and fork it).

Facts come from `src/plugins/Plugins/*/plugin.yaml`, each plugin's `backend/*.go` (adapter
spec, `ConfigFieldDecl` schema, capability flags, endpoint constants),
`src/plugins/catalog/*.json`, and the host side: `core/pluginruntime/loader.go`,
`core/plugin/contract/contract.go`, `core/plugin/host/manifest_schema.go`,
`core/plugin/host/keys.go`, `core/rbac/permissions.go`,
`modules/appstore/plugins/routes.go`, `modules/plugin_runtime/handler.go`, and the
`routes.go` of every module that owns an adapter family's admin surface.

Three findings recorded rather than smoothed over:

- `POST /api/plugins/loader/reload` carries **no** permission key. The comment above
  `plugin_runtime.Mount` says it requires `plugins.manage`; the handler does not check it
  and the route has no middleware, so any authenticated session can trigger a reload.
- `DefaultPinnedKeys` is empty by design, so with no trust keys configured the installer
  logs a warning and proceeds — signature verification is opt-in until an operator sets
  `MCHARBOR_PLUGIN_TRUST_KEYS`.
- There are no per-plugin Makefiles. `src/plugins/Makefile` iterates `Plugins/*/Makefile`,
  which matches nothing, so its `build` and `sign` targets are no-ops. The build and
  signing commands on every developer page are the ones that actually work.
