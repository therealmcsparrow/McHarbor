# Development Changelog

Development-only changes for McHarbor are documented in this file.

## [1.2.0] - 2026-05-24 (Public Release)

### Added

- Added an environments card overview with a persistent table/card switch, compact CPU and RAM sparklines, container state totals, and per-environment image update counts.
- Added authenticated user language preferences with a user-table migration, session/login payload support, a protected preferences endpoint, and profile/settings selectors that persist the selected interface language.
- Added local-user profile editing for display name and email, including a protected profile update endpoint, audit logging, localized profile form copy, and auth-store refresh after save.
- Added configured/custom delivery modes for workflow email and outbound webhook nodes, including saved webhook selection, custom SMTP settings, HMAC-signed configured webhook calls, and workflow delivery recording.
- Added configured/custom registry selection to workflow image pull, image push, and registry search nodes, including registry picker fields, default-registry fallback, and encrypted registry credential reuse.
- Added reusable workflow config field renderers for saved webhooks and registries.

### Changed

- Matched the environments card overview grid to the container cards so cards use the same responsive widths across breakpoints.
- Updated workflow node requirement inference so conditionally hidden fields do not make inactive delivery or registry modes look required.
- Hardened auth preference/session paths by checking database write and lookup errors instead of silently ignoring them.
- Fixed cron schedule previews so valid schedules with timezone labels no longer display “Invalid cron expression” because of date formatting.
- Prepared the public release metadata for `1.2.0`.

## [1.1.20-dev] - 2026-05-24

### Added

- Added configured-channel delivery selectors to workflow communication nodes for Slack, Discord, Teams, Telegram, WhatsApp, Signal, ntfy, and Gotify, with an Other mode that keeps direct custom settings available.
- Added configured/custom delivery selectors to workflow email, outbound webhook, image pull, image push, and registry search nodes, including saved webhook and registry pickers.
- Added user-bound language preferences so profile language changes are saved to the authenticated user and restored on sign-in.

### Changed

- Switched the dev branch patch version markers, Docker image defaults, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, lockfile root version, and footer display to `1.1.20-dev`.

## [1.1.19-dev] - 2026-05-24

### Added

- Added a protected local user creation endpoint with password validation, duplicate username handling, default role assignment, RBAC cache invalidation, and audit logging.
- Added a security Users tab create dialog for username/password accounts, including display name, email, default role, and active-state controls.
- Added localized create-user labels and success toast copy for every supported interface language.

### Changed

- Switched the dev branch patch version markers, Docker image defaults, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, lockfile root version, and footer display to `1.1.19-dev`.

## [1.1.18-dev] - 2026-05-24

### Added

- Added a dedicated profile page with account details, theme selection, and language selection.
- Added localized profile-page copy for every supported interface language.
- Added a Profile link to the avatar menu for direct access to account preferences.

### Changed

- Moved theme and language controls out of the avatar menu and into the profile page.
- Switched the dev branch patch version markers, Docker image defaults, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, lockfile root version, and footer display to `1.1.18-dev`.

## [1.1.17-dev] - 2026-05-24

### Added

- Added Spanish, French, Portuguese, and Chinese as fully selectable interface languages.
- Added translated frontend resource bundles for core pages, dashboard widgets, and workflow nodes in the new languages.
- Added backend API message translations and language negotiation support for `es`, `fr`, `pt`, and `zh`.

### Changed

- Rewrote the README AI section to describe how AI-assisted tooling is used for translations, reviews, documentation, and developer workflow support.
- Expanded the frontend i18n validation script to check every locale directory plus co-located widget and workflow node translation files.
- Switched the dev branch version markers, Docker image defaults, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, lockfile root version, and footer display to `1.1.17-dev`.

### Tests

- Added backend i18n tests for the new language negotiation and message registrations.

## [1.1.16-dev] - 2026-05-24

### Fixed

- Fixed container take-over so adopting a standalone container creates a durable container-to-stack link instead of only creating the managed stack record.
- Made prune-unused actions visible as page-level actions for containers, images, volumes, and stacks instead of selection-only batch actions.
- Preserved selected container rows across polling refreshes by pruning table selection only when selected row IDs disappear.

### Added

- Added manual container-to-stack relinking from container detail and stack detail views, backed by a persisted link table.

### Changed

- Updated the OpenAPI documentation for manual container-to-stack link, relink, and unlink endpoints.
- Switched the dev branch version markers, Docker image defaults, runtime metadata, OpenAPI metadata, README release reference, frontend package metadata, lockfile root version, and footer display to `1.1.16-dev`.
