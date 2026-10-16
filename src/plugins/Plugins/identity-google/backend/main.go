// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the identity-google plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the OIDC authorization-code flow against
// accounts.google.com for one provider type ("google"). It supports
// both personal Google accounts and Google Workspace (the `domain`
// field gates the latter via the `hd` query param). Group fetching
// uses the Admin SDK Directory API.
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

// googleProviderType is the ProviderType this plugin owns.
const googleProviderType = "google"

// IdentityGooglePlugin is the singleton the host dispatches to.
type IdentityGooglePlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *IdentityGooglePlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — identity-google does not register HTTP routes.
// The host's identity module owns the public /authorize and /callback
// endpoints and dispatches into the plugin's adapter at runtime.
func (p *IdentityGooglePlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op.
func (p *IdentityGooglePlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — no dashboard widgets.
func (p *IdentityGooglePlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — no workflow nodes.
func (p *IdentityGooglePlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions — no adds_permissions.
func (p *IdentityGooglePlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Google client for its single
// ProviderType. Capabilities:
//
//   OIDCRedirect       — true; Google uses the OIDC code flow
//   SupportsGroupFetch — true; Admin SDK Directory API exposes /groups
//   SAMLRedirect       — false; Google is OIDC, not SAML
//   SupportsMetadata   — false; no SP metadata document exists
func (p *IdentityGooglePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Identity().Register(
		plugin.IdentityAdapterSpec{
			ProviderType: googleProviderType,
			DisplayName:  "Google",
			Slug:         p.manifest.Slug,
			Icon:         "plugin.svg",
			Capabilities: plugin.IdentityAdapterCapabilities{
				OIDCRedirect:       true,
				SAMLRedirect:       false,
				SupportsGroupFetch: true,
				SupportsMetadata:   false,
			},
			ConfigSchema: googleConfigSchema(),
		},
		NewGoogleClient(),
	); err != nil {
		return fmt.Errorf("identity-google: register adapter: %w", err)
	}
	return nil
}

// googleConfigSchema returns the form fields the admin UI renders.
// See google_client.go for endpoint shapes.
//
//   client_id          required  string   OAuth client ID from the Google Cloud console
//   client_secret      required  secret   OAuth client secret
//   domain             optional  string   Google Workspace hosted domain (restricts login via hd=)
//   scopes             optional  string   space-separated OAuth scopes (default: openid profile email)
//   service_account_json    optional  secret   Google service account JSON for Admin SDK (group fetching)
//   delegated_user_email    optional  string   User to impersonate for Admin SDK calls
//   delegated_user_access_token optional secret   Manually-issued access token (until v1.1 JWT signing ships)
//   user_email_for_lookup  optional  string   Email of the user to look up groups for (used by FetchGroups)
func googleConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "client_id",
			Type:        "string",
			Label:       "Client ID",
			Required:    true,
			Help:        "OAuth 2.0 client ID from the Google Cloud console (APIs & Services > Credentials).",
			Placeholder: "000000000000-aaaaaaaaaaaa.apps.googleusercontent.com",
		},
		{
			Key:      "client_secret",
			Type:     "secret",
			Label:    "Client Secret",
			Required: true,
			Help:     "OAuth client secret issued by Google. Stored encrypted at rest (AES-256-GCM).",
		},
		{
			Key:         "domain",
			Type:        "string",
			Label:       "Workspace Domain",
			Required:    false,
			Help:        "Optional Google Workspace hosted domain (e.g. example.com). When set, the plugin passes it as the hd= query parameter so only accounts from that Workspace can sign in. Leave empty to allow personal Google accounts.",
			Placeholder: "example.com",
		},
		{
			Key:         "scopes",
			Type:        "string",
			Label:       "Scopes",
			Required:    false,
			Default:     "openid profile email",
			Help:        "Space-separated OAuth scopes. Defaults to openid profile email. Add https://www.googleapis.com/auth/admin.directory.group.readonly to enable admin group fetching via the Directory API.",
			Placeholder: "openid profile email",
		},
		{
			Key:      "service_account_json",
			Type:     "secret",
			Label:    "Service Account JSON (admin groups)",
			Required: false,
			Help:     "Optional: contents of a Google service-account JSON key file with domain-wide delegation. Required for Admin SDK group fetching. Stored encrypted at rest.",
		},
		{
			Key:         "delegated_user_email",
			Type:        "string",
			Label:       "Delegated User Email",
			Required:    false,
			Help:        "Optional: the Workspace admin user the service account impersonates (e.g. admin@example.com). Required when service_account_json is set.",
			Placeholder: "admin@example.com",
		},
		{
			Key:      "delegated_user_access_token",
			Type:     "secret",
			Label:    "Delegated User Access Token (override)",
			Required: false,
			Help:     "Optional: manually-issued Google API access token to use for Admin SDK calls. Use this until v1.1 ships RS256 JWT signing for service-account assertions.",
		},
		{
			Key:         "user_email_for_lookup",
			Type:        "string",
			Label:       "User Email for Group Lookup",
			Required:    false,
			Help:        "Optional: email of the user to look up groups for. Most operators leave this blank and let the host pass the authenticated user's email at runtime.",
			Placeholder: "user@example.com",
		},
	}
}

// Init stashes the host context so the Google client can audit on
// Test + Auth.
func (p *IdentityGooglePlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	setHost(host)
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.identity-google.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *IdentityGooglePlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.identity-google.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*IdentityGooglePlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up.
var Plugin plugin.Plugin = &IdentityGooglePlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "identity-google",
		Name:    "Google",
		Version: "2026.10.01",
	},
}
