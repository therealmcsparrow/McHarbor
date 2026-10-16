module github.com/therealmcsparrow/mcharbor-addons/plugins/storage-sftp

go 1.27.2

require (
	github.com/pkg/sftp v1.13.11
	github.com/therealmcsparrow/mcharbor v0.0.0
	golang.org/x/crypto v0.57.0
)

require (
	github.com/kr/fs v0.1.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// `core/plugin` is now a subpackage of the main backend module
// (see .references/backup/2026-08-14-1900-shim-core-plugin/CHANGES.md),
// so the replace directive points at the host module root rather
// than at a `core/plugin` directory.
replace github.com/therealmcsparrow/mcharbor => ../../../backend
