module github.com/therealmcsparrow/mcharbor-plugin-identity-google

go 1.27.2

require (
	github.com/therealmcsparrow/mcharbor v0.0.0
	golang.org/x/oauth2 v0.37.0
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// `core/plugin` is now a subpackage of the main backend module (see
// .references/backup/2026-08-14-1900-shim-core-plugin/CHANGES.md),
// so the replace directive points at the host module root rather
// than at a `core/plugin` directory.
replace github.com/therealmcsparrow/mcharbor => ../../../backend
