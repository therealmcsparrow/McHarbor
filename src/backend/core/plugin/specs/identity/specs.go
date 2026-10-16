// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package identity defines the identity-family adapter spec types
// (sub-registry, Spec struct, Client interface, capabilities, and
// the auth request / result / callback shapes used during OIDC /
// LDAP / SAML flow).
//
// Why split: see the package doc in `core/plugin/contract`.
//
// Imported by:
//   - host modules that bridge identity providers
//     (users, sessions, appstore auth paths)
//   - identity plugins (identity-* family — OIDC, LDAP, SAML, etc.)
package identity

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// IdentityAdapterRegistry is the sub-registry for external identity
// providers (OIDC, LDAP, SAML, …). A plugin registers one entry per
// provider type it handles. Registering the same ProviderType twice
// from the same plugin (or two different plugins) is an error — the
// host uses the ProviderType as the lookup key.
//
// The host retains the schema and CRUD surface for identity_providers
// rows and the public callback dispatcher (Phase E reversal, 2026-08-01).
// Each provider type (entra_id, google, generic_oidc, ldap, saml_2_0,
// …) ships as a plugin; the host's identity module consults the
// registry to dispatch the public auth flow at login time.
type IdentityAdapterRegistry interface {
	// Register adds an adapter. The host dispatches a provider row
	// with provider_type == spec.ProviderType to client. The client
	// must be safe for concurrent use — multiple users can hit
	// /authorize and /callback in parallel.
	Register(spec IdentityAdapterSpec, client IdentityAdapterClient) error
}

// IdentityAdapterSpec describes one identity provider type a plugin
// handles. The host uses the spec to render the admin UI's
// provider_type dropdown and to dispatch the public auth flow at
// login time.
type IdentityAdapterSpec struct {
	// ProviderType is the unique key the host uses to dispatch
	// providers. Must match a value stored in the host's
	// identity_providers.provider_type column. Common values include
	// "entra_id", "google", "generic_oidc", "ldap", "saml_2_0".
	ProviderType string

	// DisplayName is the user-facing label shown in the admin UI's
	// provider_type dropdown and on the login page.
	DisplayName string

	// Slug is the owning plugin's slug (e.g. "identity-entra-id").
	// The host surfaces it alongside the spec so the frontend can
	// resolve icon assets via /api/plugins/loader/icon without a
	// second round-trip. Plugins populate this in RegisterAdapters.
	Slug string

	// Icon is the asset filename inside the plugin's icons/
	// directory (resolved by the host's /api/plugins/loader/icon
	// route at render time). Empty string is valid — renderers
	// fall back to their default tile (IconShieldLock, etc.).
	// The host trusts the value because it is declared in the
	// plugin's own plugin.yaml — an attacker who can edit a
	// plugin's manifest already controls the .so bundle.
	Icon string

	// Capabilities declares which operations the adapter supports.
	// The host hides unsupported actions in the admin UI and short-
	// circuits unsupported runtime calls with ErrUnsupported.
	Capabilities IdentityAdapterCapabilities

	// ConfigSchema is the optional list of custom form fields the
	// admin UI renders when an operator picks this provider type.
	// The host already knows about the per-type columns on
	// identity_providers (tenant_id, domain, issuer_url, …); plugins
	// extend the schema by adding custom keys.
	ConfigSchema []types.ConfigFieldDecl
}

// IdentityAdapterCapabilities declares what the adapter supports.
// The host consults these before invoking the corresponding method
// on IdentityAdapterClient.
type IdentityAdapterCapabilities struct {
	// OIDCRedirect indicates the adapter implements OIDC's
	// authorization-code redirect dance (BeginAuth returning a
	// redirect URL and HandleCallback exchanging the code).
	OIDCRedirect bool

	// SAMLRedirect indicates the adapter implements SAML's
	// redirect / POST binding dance (BeginAuth returning a redirect
	// URL or HTML form, HandleCallback consuming the assertion).
	SAMLRedirect bool

	// SupportsGroupFetch indicates the adapter implements
	// FetchGroups for admin UI group-mapping workflows. Providers
	// that don't expose a groups API (LDAP via simple bind, generic
	// OIDC) leave this false.
	SupportsGroupFetch bool

	// SupportsMetadata indicates the adapter implements
	// MetadataXML for SP-initiated SAML / metadata discovery.
	SupportsMetadata bool
}

// IdentityAuthRequest is the context the host hands to a plugin when
// the public /authorize endpoint is invoked. Plugins build either a
// redirect URL (OIDC / SAML redirect binding) or an HTML form (SAML
// POST binding).
type IdentityAuthRequest struct {
	// ProviderID is the host's row ID for the configured provider.
	// Plugins persist state (state token, relay state, SAML request
	// ID) keyed by ProviderID.
	ProviderID string

	// BaseURL is the externally-visible origin of the host
	// (scheme + host + port). Plugins use it to construct the
	// redirect_uri / ACS URL they hand to the upstream provider.
	BaseURL string
}

// IdentityAuthResult is what BeginAuth returns to the host. The host
// uses either RedirectURL (HTTP 302) or HTMLForm (auto-submitting
// HTML page) to forward the browser to the upstream provider.
type IdentityAuthResult struct {
	// RedirectURL is set when the upstream provider uses an HTTP
	// redirect binding (OIDC, SAML HTTP-Redirect). The host responds
	// with a 302 to this URL.
	RedirectURL string

	// HTMLForm is set when the upstream provider uses HTTP-POST
	// binding (some SAML IdPs). The host returns it as an
	// auto-submitting HTML page.
	HTMLForm string
}

// IdentityCallbackRequest is the context the host hands to a plugin
// during the public /callback (OIDC) or /acs (SAML) flow. Plugins
// validate the upstream response, exchange the code / parse the
// assertion, and return the externally-asserted user identity.
type IdentityCallbackRequest struct {
	// ProviderID is the host's row ID for the configured provider.
	ProviderID string

	// BaseURL is the externally-visible origin of the host.
	BaseURL string

	// Code is the OIDC authorization code (OIDC providers).
	Code string

	// State is the OIDC state token (OIDC providers). Plugins should
	// reject mismatched / expired states.
	State string

	// Form contains the SAML POST body for SAML providers. For
	// HTTP-Redirect binding the relevant fields land in Query.
	Form map[string]string

	// Query holds the SAML RelayState and any other URL parameters
	// from the HTTP-Redirect binding.
	Query map[string]string

	// RawRequest is the underlying HTTP request for SAML providers
	// that need full access to the body (e.g., XML-in-form parsing).
	// nil when the callback originated from an OIDC redirect.
	RawRequest *http.Request
}

// IdentityCallbackResult is what HandleCallback returns. The host
// then calls into its provisioning logic to create or update the
// internal user, and issues a session cookie.
type IdentityCallbackResult struct {
	// Subject is the upstream provider's unique identifier for the
	// user (OIDC `sub` claim, SAML NameID). Required.
	Subject string

	// Email is the upstream-asserted email address. Optional but
	// recommended — the host uses it as a stable matching key when
	// linking external identities to existing local users.
	Email string

	// DisplayName is the upstream-asserted display name. Optional.
	DisplayName string

	// Groups is the list of upstream-asserted group / role names.
	// The host applies group-mapping / auto-import rules.
	Groups []string

	// Username is the upstream-asserted preferred username. When
	// non-empty the host uses it as the McHarbor username; otherwise
	// the host falls back to email or a generated username.
	Username string

	// ProviderSlug is the plugin's slug (e.g. "identity-oidc"). The
	// host uses it to choose a default username prefix when one is
	// generated. Required.
	ProviderSlug string
}

// IdentityAdapterClient is the runtime contract for plugin-provided
// identity providers. The host's modules/identity dispatcher calls
// these methods from the public /authorize and /callback endpoints.
//
// All methods receive a config map carrying the provider's row
// columns (provider_type, client_id, tenant_id, issuer_url, …) keyed
// by column name. Plugins decide which keys they need.
//
// Implementations MUST be safe for concurrent use. Multiple users can
// hit /authorize and /callback in parallel; the host invokes the
// adapter from multiple goroutines.
type IdentityAdapterClient interface {
	// Test verifies the provider is reachable and the credentials
	// work. The host calls this from the admin UI's "Test connection"
	// button. Implementations should validate every config field
	// they will use at runtime and return a descriptive error on
	// failure.
	Test(ctx context.Context, config map[string]string) error

	// BeginAuth initiates the upstream auth flow. The host calls
	// this from the public /authorize endpoint when a user clicks
	// "Sign in with <ProviderName>" on the login page. The plugin
	// must persist any state (state token, SAML request ID, …)
	// keyed by ProviderID so HandleCallback can validate the
	// returning request.
	BeginAuth(ctx context.Context, config map[string]string, req IdentityAuthRequest) (*IdentityAuthResult, error)

	// HandleCallback completes the upstream auth flow. The host
	// calls this from the public /callback (OIDC) or /acs (SAML)
	// endpoint after the upstream provider redirects the user
	// back. The plugin validates the response, extracts the
	// upstream-asserted user identity, and returns it. The host
	// then runs its own provisioning + session-issuance logic.
	HandleCallback(ctx context.Context, config map[string]string, req IdentityCallbackRequest) (*IdentityCallbackResult, error)

	// FetchGroups lists upstream-side groups. The host calls this
	// from the admin UI's group-mapping wizard. Adapters that
	// declared SupportsGroupFetch=false return ErrUnsupported.
	FetchGroups(ctx context.Context, config map[string]string) ([]IdentityGroup, error)

	// MetadataXML returns the SP-side metadata document for SAML
	// providers. The host calls this from the public /metadata
	// endpoint. Adapters that declared SupportsMetadata=false
	// return ErrUnsupported.
	MetadataXML(ctx context.Context, config map[string]string, baseURL string) ([]byte, error)
}

// IdentityGroup describes a single group returned by
// IdentityAdapterClient.FetchGroups.
type IdentityGroup struct {
	ID          string
	Name        string
	Description string
}
