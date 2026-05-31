# Changelog

All notable changes to McHarbor are documented in this file.

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
