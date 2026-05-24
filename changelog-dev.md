# Development Changelog

Development-only changes for McHarbor are documented in this file.

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
