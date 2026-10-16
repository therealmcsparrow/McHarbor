// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the identity-saml plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the SAML 2.0 Service-Provider flow for one
// provider type ("saml_2_0") — building the SP, generating
// AuthnRequests via the IdP's metadata, parsing signed assertions at
// /acs, and emitting SP metadata at /metadata. Group fetching via
// SAML is a v1.1 feature (FetchGroups returns ErrUnsupported).
//
// The host's modules/identity dispatcher calls into this adapter from
// the public /authorize, /callback, and /metadata endpoints after the
// Phase 3 host-slim refactor.
package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// samlProviderType is the ProviderType this plugin owns.
const samlProviderType = "saml_2_0"

// IdentitySamlPlugin is the singleton the host dispatches to.
type IdentitySamlPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *IdentitySamlPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — identity-saml does not register HTTP routes.
// The host's identity module owns the public /authorize, /callback,
// and /metadata endpoints and dispatches into the plugin's adapter
// at runtime.
func (p *IdentitySamlPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op.
func (p *IdentitySamlPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — no dashboard widgets.
func (p *IdentitySamlPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — no workflow nodes.
func (p *IdentitySamlPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions — no adds_permissions.
func (p *IdentitySamlPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the SAML client for its single
// ProviderType.
//
// Capabilities:
//   OIDCRedirect       — false; SAML is not OIDC
//   SAMLRedirect       — true; HTTPPostBinding flow
//   SupportsGroupFetch — false; SAML group claim is not exposed as a directory API
//   SupportsMetadata   — true; the plugin emits SP metadata XML
func (p *IdentitySamlPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Identity().Register(
		plugin.IdentityAdapterSpec{
			ProviderType: samlProviderType,
			DisplayName:  "SAML 2.0",
			Slug:         p.manifest.Slug,
			Icon:         "plugin.svg",
			Capabilities: plugin.IdentityAdapterCapabilities{
				OIDCRedirect:       false,
				SAMLRedirect:       true,
				SupportsGroupFetch: false,
				SupportsMetadata:   true,
			},
			ConfigSchema: samlConfigSchema(),
		},
		NewSamlClient(),
	); err != nil {
		return fmt.Errorf("identity-saml: register adapter: %w", err)
	}
	return nil
}

// samlConfigSchema returns the form fields the admin UI renders
// when an operator picks this ProviderType.
//
// Fields summary (see saml_client.go for endpoint shapes):
//
//   metadata_url   required  string   IdP metadata XML URL
//   entity_id      optional  string   SP entity ID (defaults to MetadataURL)
//   sp_certificate optional  secret   SP PEM-encoded X.509 cert
//   sp_private_key optional  secret   SP PEM-encoded private key
func samlConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "metadata_url",
			Type:        "string",
			Label:       "IdP Metadata URL",
			Required:    true,
			Help:        "URL of the IdP's SAML 2.0 metadata XML document. The plugin fetches this to discover the IdP's SSO endpoint + signing certificate.",
			Placeholder: "https://idp.example.com/saml/metadata",
		},
		{
			Key:         "entity_id",
			Type:        "string",
			Label:       "SP Entity ID",
			Required:    false,
			Help:        "Optional SP entity ID. Leave empty to use the McHarbor metadata URL. Must match what's configured on the IdP side.",
			Placeholder: "https://mcharbor.example.com",
		},
		{
			Key:      "sp_certificate",
			Type:     "secret",
			Label:    "SP Certificate (PEM)",
			Required: false,
			Help:     "Optional PEM-encoded X.509 certificate. Required if the IdP requires signed AuthnRequests / assertions. Generate with `openssl req -x509 -newkey rsa:2048 -keyout sp.key -out sp.crt -days 365 -nodes -subj '/CN=mcharbor'`. Stored encrypted at rest.",
		},
		{
			Key:      "sp_private_key",
			Type:     "secret",
			Label:    "SP Private Key (PEM)",
			Required: false,
			Help:     "Optional PEM-encoded RSA private key matching the SP certificate. Stored encrypted at rest.",
		},
	}
}

// Init stashes the host context so the SAML client can audit on
// Test + Auth.
func (p *IdentitySamlPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.identity-saml.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *IdentitySamlPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.identity-saml.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*IdentitySamlPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up.
var Plugin plugin.Plugin = &IdentitySamlPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "identity-saml",
		Name:    "SAML 2.0",
		Version: "2026.10.01",
	},
}
