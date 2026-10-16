// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the identity-entra-id plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the OIDC authorization-code flow for one
// provider type ("entra_id") and the Microsoft Graph API group fetch
// for the host's admin UI group-mapping wizard. The plugin implements
// plugin.IdentityAdapterClient (Test + BeginAuth + HandleCallback +
// FetchGroups). MetadataXML returns ErrUnsupported because Entra ID
// is OIDC, not SAML — no SP metadata document exists.
//
// The host's modules/identity dispatcher calls into this adapter
// from the public /authorize and /callback endpoints after the
// Phase 3 host-slim refactor. Until then, the host still uses its
// legacy per-type OIDC code; the plugin loads and registers but
// isn't dispatched at runtime.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// entraProviderType is the ProviderType this plugin owns.
// Single value — the host's identity_providers.provider_type
// column accepts this string.
const entraProviderType = "entra_id"

// IdentityEntraIDPlugin is the singleton the host dispatches to.
// It owns one ProviderType's worth of IdentityAdapterSpecs and
// routes host calls into entraClient.
type IdentityEntraIDPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *IdentityEntraIDPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — identity-entra-id does not register HTTP routes.
// The host's identity module owns the public /authorize, /callback,
// and /metadata endpoints and dispatches into the plugin's adapter
// at runtime. No plugin-owned routes are needed for v1.
func (p *IdentityEntraIDPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Identity providers are
// managed from the host's Settings > Security > Identity Providers
// page, which lists every registered ProviderType (built-in + plugin).
// A dedicated sidemenu entry would just duplicate the surface.
func (p *IdentityEntraIDPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *IdentityEntraIDPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *IdentityEntraIDPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on every
// permission check; this plugin declares no adds_permissions, so
// this is a no-op.
func (p *IdentityEntraIDPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Entra ID client for its single
// ProviderType. The cfg map the host hands to Test/BeginAuth/etc.
// carries the provider row's columns keyed by ConfigFieldDecl.Key.
//
// Capabilities:
//   OIDCRedirect       — true; Entra ID uses the OIDC code flow
//   SupportsGroupFetch — true; the Graph API exposes /groups
//   SAMLRedirect       — false; Entra ID is OIDC, not SAML
//   SupportsMetadata   — false; no SP metadata document exists
//
// The provider_type dropdown + admin UI tile render the DisplayName
// from this spec; the slug (used to look up icons + the UMD bundle)
// is read from the plugin's own manifest by the host's loader, not
// from this spec field — IdentityAdapterSpec intentionally omits Slug
// because the loader has the canonical value.
func (p *IdentityEntraIDPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Identity().Register(
		plugin.IdentityAdapterSpec{
			ProviderType: entraProviderType,
			DisplayName:  "Microsoft Entra ID",
			Slug:         p.manifest.Slug,
			Icon:         "plugin.svg",
			Capabilities: plugin.IdentityAdapterCapabilities{
				OIDCRedirect:       true,
				SAMLRedirect:       false,
				SupportsGroupFetch: true,
				SupportsMetadata:   false,
			},
			ConfigSchema: entraConfigSchema(),
		},
		NewEntraClient(),
	); err != nil {
		return fmt.Errorf("identity-entra-id: register adapter: %w", err)
	}
	return nil
}

// entraConfigSchema returns the form fields the admin UI renders
// when an operator picks this ProviderType. The schema is also
// the union of fields the runtime methods (Test, BeginAuth,
// HandleCallback, FetchGroups) read from the cfg map.
//
// Fields summary (see entra_client.go for endpoint shapes):
//
//   tenant_id      required  string   Azure AD tenant ID (UUID or "common")
//   client_id      required  string   App registration's application (client) ID
//   client_secret  required  secret   App registration's client secret value
//   scopes         optional  string   space-separated scopes (default: openid profile email)
func entraConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "tenant_id",
			Type:        "string",
			Label:       "Tenant ID",
			Required:    true,
			Help:        "Azure AD tenant ID (UUID). Use 'common' for multi-tenant apps, 'organizations' for any work/school account, or leave 'common' for personal accounts too.",
			Placeholder: "00000000-0000-0000-0000-000000000000",
		},
		{
			Key:         "client_id",
			Type:        "string",
			Label:       "Client ID",
			Required:    true,
			Help:        "Application (client) ID from the App Registration overview page.",
			Placeholder: "11111111-1111-1111-1111-111111111111",
		},
		{
			Key:      "client_secret",
			Type:     "secret",
			Label:    "Client Secret",
			Required: true,
			Help:     "Client secret value from the App Registration's Certificates & secrets page. Stored encrypted at rest.",
		},
		{
			Key:      "scopes",
			Type:     "string",
			Label:    "Scopes",
			Required: false,
			Default:  "openid profile email",
			Help:     "Space-separated OAuth scopes. The Microsoft Graph default is 'https://graph.microsoft.com/.default' when group fetching is enabled.",
			Placeholder: "openid profile email",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Entra ID client can audit on Test + Auth.
func (p *IdentityEntraIDPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.identity-entra-id.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *IdentityEntraIDPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.identity-entra-id.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*IdentityEntraIDPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &IdentityEntraIDPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "identity-entra-id",
		Name:    "Microsoft Entra ID",
		Version: "2026.10.01",
	},
}
