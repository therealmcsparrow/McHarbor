# Changelog

All notable changes to McHarbor-AddOns are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Initial bootstrap of the repo. See `.references/plugins/architecture.md`
  in the McHarbor checkout for the design.
- `plugin-sdk/` vendored copy of the host's `core/plugin` package —
  removed once the host ships a tagged release.
- `Plugins/hello-world/` — minimal plugin that exercises every hook
  in the contract (routes, menu, widget, i18n, migrations).
- `Apps/nginx/` and `Apps/postgres/` — example Compose app templates.
- `scripts/sign-index.py` + `scripts/verify-index.py` for catalog signing.
- AGENTS.md contributor guide.