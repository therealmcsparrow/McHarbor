module github.com/therealmcsparrow/mcharbor-plugin-identity-saml

go 1.27.2

require (
	github.com/crewjam/saml v0.5.1
	github.com/therealmcsparrow/mcharbor v0.0.0
)

require (
	github.com/beevik/etree v1.7.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/mattermost/xml-roundtrip-validator v0.1.0 // indirect
	github.com/russellhaering/goxmldsig v1.6.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/crypto v0.57.0 // indirect
)

// `core/plugin` is now a subpackage of the main backend module (see
// .references/backup/2026-08-14-1900-shim-core-plugin/CHANGES.md),
// so the replace directive points at the host module root rather
// than at a `core/plugin` directory.
replace github.com/therealmcsparrow/mcharbor => ../../../backend
