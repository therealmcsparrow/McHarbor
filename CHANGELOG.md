# Changelog

All notable changes to McHarbor are documented in this file.

## [1.1.1] - 2026-05-23

### Changed

- Bumped the McHarbor Docker image, agent image, runtime metadata, and frontend package version metadata to `1.1.1`.

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
