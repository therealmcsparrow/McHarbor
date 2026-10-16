module github.com/therealmcsparrow/mcharbor/plugins/snapshot-truenas

go 1.27.2

require github.com/therealmcsparrow/mcharbor v0.0.0

// `core/plugin` is now a subpackage of the main backend module
// (see .references/backup/2026-08-14-1900-shim-core-plugin/CHANGES.md),
// so the replace directive points at the host module root rather
// than at a `core/plugin` directory.
replace github.com/therealmcsparrow/mcharbor => ../../../backend
