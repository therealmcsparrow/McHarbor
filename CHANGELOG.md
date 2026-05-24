# Changelog

All notable changes to McHarbor are documented in this file.

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
