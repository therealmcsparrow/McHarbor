# Changelog

All notable changes to McHarbor are documented in this file.

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
