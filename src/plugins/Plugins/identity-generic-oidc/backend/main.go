// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the identity-generic-oidc plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the OIDC authorization-code flow for one
// provider type ("generic_oidc") — any compliant OIDC issuer whose
// discovery document lives at <issuer>/.well-known/openid-configuration
// (Auth0, Okta, Keycloak, AWS Cognito, Ping, ForgeRock, LemonLDAP, etc.).
// The plugin implements plugin.IdentityAdapterClient (Test + BeginAuth +
// HandleCallback + FetchGroups). FetchGroups returns ErrUnsupported
// because every IdP exposes groups differently; operators should map
// the OIDC-asserted "groups" claim instead. MetadataXML returns
// ErrUnsupported because generic OIDC is OIDC, not SAML — no SP
// metadata document exists.
//
// The host's modules/identity dispatcher calls into this adapter
// from the public /authorize and /callback endpoints after the
// Phase 3 host-slim refactor.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// genericOidcProviderType is the ProviderType this plugin owns.
// Single value — the host's identity_providers.provider_type
// column accepts this string.
const genericOidcProviderType = "generic_oidc"

// IdentityGenericOidcPlugin is the singleton the host dispatches to.
// It owns one ProviderType's worth of IdentityAdapterSpecs and
// routes host calls into genericOidcClient.
type IdentityGenericOidcPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *IdentityGenericOidcPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — identity-generic-oidc does not register HTTP routes.
// The host's identity module owns the public /authorize, /callback,
// and /metadata endpoints and dispatches into the plugin's adapter
// at runtime. No plugin-owned routes are needed for v1.
func (p *IdentityGenericOidcPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Identity providers are
// managed from the host's Settings > Security > Identity Providers
// page, which lists every registered ProviderType (built-in + plugin).
// A dedicated sidemenu entry would just duplicate the surface.
func (p *IdentityGenericOidcPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *IdentityGenericOidcPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *IdentityGenericOidcPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on every
// permission check; this plugin declares no adds_permissions, so
// this is a no-op.
func (p *IdentityGenericOidcPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Generic OIDC client for its single
// ProviderType. The cfg map the host hands to Test/BeginAuth/etc.
// carries the provider row's columns keyed by ConfigFieldDecl.Key.
//
// Capabilities:
//   OIDCRedirect       — true; generic OIDC uses the OIDC code flow
//   SupportsGroupFetch — false; no admin-side groups API common to all IdPs
//   SAMLRedirect       — false; generic OIDC is OIDC, not SAML
//   SupportsMetadata   — false; no SP metadata document exists
//
// The provider_type dropdown + admin UI tile render the DisplayName
// from this spec; the slug (used to look up icons + the UMD bundle)
// is read from the plugin's own manifest by the host's loader, not
// from this spec field — IdentityAdapterSpec intentionally omits Slug
// because the loader has the canonical value.
func (p *IdentityGenericOidcPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Identity().Register(
		plugin.IdentityAdapterSpec{
			ProviderType: genericOidcProviderType,
			DisplayName:  "Generic OpenID Connect",
			Slug:         p.manifest.Slug,
			Icon:         "plugin.svg",
			Capabilities: plugin.IdentityAdapterCapabilities{
				OIDCRedirect:       true,
				SAMLRedirect:       false,
				SupportsGroupFetch: false,
				SupportsMetadata:   false,
			},
			ConfigSchema: genericOidcConfigSchema(),
		},
		NewGenericOidcClient(),
	); err != nil {
		return fmt.Errorf("identity-generic-oidc: register adapter: %w", err)
	}
	return nil
}

// genericOidcConfigSchema returns the form fields the admin UI renders
// when an operator picks this ProviderType. The schema is also
// the union of fields the runtime methods (Test, BeginAuth,
// HandleCallback, FetchGroups) read from the cfg map.
//
// Fields summary (see generic_oidc_client.go for endpoint shapes):
//
//   issuer_url    required  string   OIDC issuer URL (the host fetches
//                                    <issuer>/.well-known/openid-configuration)
//   client_id     required  string   OAuth client ID
//   client_secret required  secret   OAuth client secret
//   scopes        optional  string   space-separated scopes (default: openid profile email)
func genericOidcConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "issuer_url",
			Type:        "string",
			Label:       "Issuer URL",
			Required:    true,
			Help:        "The OIDC issuer URL. The plugin fetches <issuer_url>/.well-known/openid-configuration to discover authorization and token endpoints.",
			Placeholder: "https://login.example.com",
		},
		{
			Key:         "client_id",
			Type:        "string",
			Label:       "Client ID",
			Required:    true,
			Help:        "OAuth client ID assigned by your IdP to the McHarbor application.",
			Placeholder: "mcharbor-client",
		},
		{
			Key:      "client_secret",
			Type:     "secret",
			Label:    "Client Secret",
			Required: true,
			Help:     "OAuth client secret issued by your IdP. Stored encrypted at rest (AES-256-GCM).",
		},
		{
			Key:         "scopes",
			Type:        "string",
			Label:       "Scopes",
			Required:    false,
			Default:     "openid profile email",
			Help:        "Space-separated OAuth scopes. The default covers standard OIDC claims. Add 'groups' if your IdP exposes a group claim under that name.",
			Placeholder: "openid profile email groups",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Generic OIDC client can audit on Test + Auth.
func (p *IdentityGenericOidcPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	setHost(host)
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.identity-generic-oidc.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *IdentityGenericOidcPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.identity-generic-oidc.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*IdentityGenericOidcPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &IdentityGenericOidcPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "identity-generic-oidc",
		Name:    "Generic OpenID Connect",
		Version: "2026.10.01",
	},
}
