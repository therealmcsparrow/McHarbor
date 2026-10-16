module github.com/therealmcsparrow/mcharbor-plugin-identity-ldap

go 1.27.2

require (
	github.com/go-ldap/ldap/v3 v3.4.14
	github.com/therealmcsparrow/mcharbor v0.0.0
)

require (
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/crypto v0.57.0 // indirect
)

// `core/plugin` is now a subpackage of the main backend module (see
// .references/backup/2026-08-14-1900-shim-core-plugin/CHANGES.md),
// so the replace directive points at the host module root rather
// than at a `core/plugin` directory.
replace github.com/therealmcsparrow/mcharbor => ../../../backend
