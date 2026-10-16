// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the identity-ldap plugin. It exports
// a single symbol — `Plugin` — that the host loads via `plugin.Open`
// and dispatches to.
//
// Scope: this plugin owns the LDAP bind-and-search authentication
// flow for one provider type ("ldap"). The plugin implements
// plugin.IdentityAdapterClient (Test + FetchGroups only; the
// redirect-flow methods return ErrUnsupported). The plugin also
// registers one HTTP route — POST /api/identity-ldap/auth
// — that accepts the form-post username/password and performs the
// search-then-rebind. Everything around the bind — lockout, account
// provisioning, MFA, the session cookie and the reply — is the host's,
// reached through the externalLoginHost capability on host.Identity.
//
// LDAP has its own per-provider config (bind_dn, user_base_dn,
// start_tls, ...) that doesn't fit the unified `identity_providers`
// schema's typed columns. v1 folds everything into the host's
// config map, so the plugin doesn't own a table.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ldapProviderType is the ProviderType this plugin owns. Single
// value — the host's identity_providers.provider_type column
// accepts this string.
const ldapProviderType = "ldap"

// ldapAuthPath is the form-post login route. It lives under the plugin's
// slug because the host's /api/identity-providers subtree answers 404 for
// any path it does not own, which is how the first address shipped dead.
const ldapAuthPath = "/api/identity-ldap/auth"

// IdentityLdapPlugin is the singleton the host dispatches to. It
// owns one ProviderType's worth of IdentityAdapterSpecs and the
// form-based auth route.
type IdentityLdapPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	// dir is the directory the sign-in route binds against; nil means the real
	// network directory, and tests inject a fake.
	dir ldapDirectory
}

// Manifest returns the parsed plugin.yaml.
func (p *IdentityLdapPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes registers the single form-based auth endpoint. It is
// permission-free because plugin.yaml declares it public, and registered in
// the Auth class so it gets the same per-client rate limit as /api/auth/login.
func (p *IdentityLdapPlugin) RegisterRoutes(reg plugin.Registry) error {
	reg.Auth(http.MethodPost, ldapAuthPath, p.handleAuth)
	return nil
}

// RegisterMenu / RegisterWidgets / RegisterNodes / RegisterPermissions
// are no-ops. The host renders the LDAP provider list under its
// Identity Providers settings tab.
func (p *IdentityLdapPlugin) RegisterMenu(_ plugin.MenuRegistry) error       { return nil }
func (p *IdentityLdapPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error  { return nil }
func (p *IdentityLdapPlugin) RegisterNodes(_ plugin.NodeRegistry) error     { return nil }
func (p *IdentityLdapPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers one IdentityAdapterClient for the
// "ldap" provider type. The adapter implements Test and
// FetchGroups; the redirect-flow methods return ErrUnsupported.
func (p *IdentityLdapPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Identity().Register(
		plugin.IdentityAdapterSpec{
			ProviderType: ldapProviderType,
			DisplayName:  "LDAP / Active Directory",
			Slug:         p.manifest.Slug,
			Icon:         "plugin.svg",
			Capabilities: plugin.IdentityAdapterCapabilities{
				OIDCRedirect:       false,
				SAMLRedirect:       false,
				SupportsGroupFetch: true,
				SupportsMetadata:   false,
			},
			ConfigSchema: ldapConfigSchema(),
		},
		&ldapAdapter{plugin: p},
	); err != nil {
		return fmt.Errorf("identity-ldap: register adapter: %w", err)
	}
	return nil
}

// ldapConfigSchema returns the form fields the admin UI renders
// when an operator picks this ProviderType. The schema is also the
// union of fields the runtime methods (Test, FetchGroups) read
// from the cfg map.
//
// Fields summary:
//
//   url              required  string   LDAP server URL (ldap://host:389 or ldaps://host:636)
//   bind_dn          optional  string   service-account DN to bind for searches; empty = anonymous
//   bind_password    optional  secret   service-account password; stored encrypted at rest
//   user_base_dn     required  string   base DN under which user entries live
//   user_filter      optional  string   search filter for user lookup; default "(uid=%s)"
//   user_attr        optional  string   attribute used as username; default "uid" (AD: "sAMAccountName")
//   email_attr       optional  string   attribute that holds the email; default "mail"
//   name_attr        optional  string   attribute that holds the display name; default "cn"
//   start_tls        optional  bool     upgrade plain ldap:// to TLS via StartTLS; default false
//   skip_tls_verify  optional  bool     skip TLS certificate verification; default false (TEST ONLY)
//   group_base_dn    optional  string   base DN for group search; defaults to user_base_dn
//   group_filter     optional  string   filter template for groups; default "(|(member=%s)(uniqueMember=%s))"
//   group_attr       optional  string   attribute used as group name; default "cn"
func ldapConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "url",
			Type:        "string",
			Label:       "LDAP Server URL",
			Required:    true,
			Help:        "LDAP server URL: ldap://host:389 for plain, ldaps://host:636 for implicit TLS. Use StartTLS instead of LDAPS when the server listens on 389.",
			Placeholder: "ldaps://ldap.example.com:636",
		},
		{
			Key:         "bind_dn",
			Type:        "string",
			Label:       "Bind DN (service account)",
			Help:        "DN of a service account used to search for users and groups. Empty = anonymous bind (most directories refuse to return user entries anonymously). Example: cn=mcharbor-svc,ou=service,dc=example,dc=com.",
			Placeholder: "cn=mcharbor,ou=service,dc=example,dc=com",
		},
		{
			Key:      "bind_password",
			Type:     "secret",
			Label:    "Bind Password",
			Help:     "Password for the service account. Stored encrypted at rest (AES-256-GCM).",
		},
		{
			Key:         "user_base_dn",
			Type:        "string",
			Label:       "User Base DN",
			Required:    true,
			Help:        "Base DN under which user entries live. Searches for incoming usernames are scoped to this subtree. Example: ou=users,dc=example,dc=com.",
			Placeholder: "ou=users,dc=example,dc=com",
		},
		{
			Key:         "user_filter",
			Type:        "string",
			Label:       "User Search Filter",
			Help:        "LDAP filter template for looking up a user by their login name. The placeholder %s is replaced with the escaped username. Default (uid=%s) works for OpenLDAP; for Active Directory use (sAMAccountName=%s) or (&(objectCategory=person)(objectClass=user)(sAMAccountName=%s)).",
			Placeholder: "(uid=%s)",
		},
		{
			Key:         "user_attr",
			Type:        "string",
			Label:       "Username Attribute",
			Help:        "Attribute that contains the login name. Default: uid (OpenLDAP). Active Directory: sAMAccountName.",
			Placeholder: "uid",
		},
		{
			Key:         "email_attr",
			Type:        "string",
			Label:       "Email Attribute",
			Help:        "Attribute that contains the user's email address. Default: mail.",
			Placeholder: "mail",
		},
		{
			Key:         "name_attr",
			Type:        "string",
			Label:       "Display Name Attribute",
			Help:        "Attribute that contains the user's display name. Default: cn.",
			Placeholder: "cn",
		},
		{
			Key:      "start_tls",
			Type:     "boolean",
			Label:    "Use StartTLS",
			Help:     "When the URL is ldap://, upgrade the connection to TLS via the StartTLS extended operation. Has no effect when the URL is already ldaps://.",
		},
		{
			Key:      "skip_tls_verify",
			Type:     "boolean",
			Label:    "Skip TLS Verification",
			Help:     "Skip certificate verification. Test/development only — do NOT enable in production.",
		},
		{
			Key:         "group_base_dn",
			Type:        "string",
			Label:       "Group Base DN",
			Help:        "Base DN for group searches. Defaults to User Base DN. Example: ou=groups,dc=example,dc=com.",
			Placeholder: "ou=groups,dc=example,dc=com",
		},
		{
			Key:         "group_filter",
			Type:        "string",
			Label:       "Group Filter",
			Help:        "Filter template for fetching groups a user belongs to. The %s placeholder is replaced with the user's full DN. Default: (|(member=%s)(uniqueMember=%s)).",
			Placeholder: "(|(member=%s)(uniqueMember=%s))",
		},
		{
			Key:         "group_attr",
			Type:        "string",
			Label:       "Group Name Attribute",
			Help:        "Attribute used as the group name in McHarbor's group mapper. Default: cn.",
			Placeholder: "cn",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host context
// so the auth route can reach the host's external-login capability.
func (p *IdentityLdapPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.identity-ldap.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *IdentityLdapPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.identity-ldap.shutdown", nil)
	}
	return nil
}

// --- form-based auth endpoint -------------------------------------
//
// This is the public POST endpoint the login page targets. The plugin checks
// the credential against the directory; the host does everything else a
// password login does, through the externalLoginHost capability.

type ldapAuthRequest struct {
	ProviderID string `json:"providerId"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// handleAuth runs one directory sign-in in the order that keeps lockout honest:
// the address gate before any directory contact, the account gate before the
// user's bind, and every rejected bind reported back so it counts.
func (p *IdentityLdapPlugin) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req ldapAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResp("invalid_body", "invalid request body"))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	// An empty password is refused here and never reaches Bind: most directories
	// treat a DN with an empty password as a successful unauthenticated bind.
	if req.ProviderID == "" || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("invalid_input", "providerId, username, and password are required"))
		return
	}

	host, ok := p.loginHost()
	if !ok {
		p.host.Logger.Error("identity-ldap: host has no external-login capability; update McHarbor to use LDAP sign-in")
		writeJSON(w, http.StatusServiceUnavailable, errorResp("host_unsupported", "this McHarbor version cannot complete an LDAP sign-in"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := host.BeginExternalLogin(ctx, w, r, req.ProviderID, req.Username); err != nil {
		return
	}

	cfg, err := host.LDAPProviderConfig(ctx, req.ProviderID)
	if err != nil {
		p.host.Logger.Warn("identity-ldap: provider unavailable", "provider_id", req.ProviderID, "error", err)
		host.FailExternalLogin(ctx, w, r, req.ProviderID, req.Username, "", reasonUnavailable)
		return
	}
	prov := ldapConfigFromMap(cfg).withDefaults()

	session, err := p.directory().Open(prov)
	if err != nil {
		p.host.Logger.Warn("identity-ldap: directory unreachable", "provider_id", req.ProviderID, "error", err)
		host.FailExternalLogin(ctx, w, r, req.ProviderID, req.Username, "", reasonUnavailable)
		return
	}
	defer session.Close()

	entry, err := session.FindUser(req.Username)
	if err != nil {
		if errors.Is(err, errDirectoryUserNotFound) {
			host.FailExternalLogin(ctx, w, r, req.ProviderID, req.Username, "", reasonInvalidCredentials)
			return
		}
		p.host.Logger.Warn("identity-ldap: user lookup failed", "provider_id", req.ProviderID, "error", err)
		host.FailExternalLogin(ctx, w, r, req.ProviderID, req.Username, "", reasonUnavailable)
		return
	}

	if err := host.CheckExternalAccount(ctx, w, r, req.ProviderID, req.Username, entry.DN, entry.Email); err != nil {
		return
	}

	if err := session.Authenticate(entry.DN, req.Password); err != nil {
		reason := reasonInvalidCredentials
		if !errors.Is(err, errDirectoryInvalidCredentials) {
			reason = reasonUnavailable
			p.host.Logger.Warn("identity-ldap: user bind failed", "provider_id", req.ProviderID, "error", err)
		}
		host.FailExternalLogin(ctx, w, r, req.ProviderID, req.Username, entry.DN, reason)
		return
	}

	loginName := entry.Username
	if loginName == "" {
		loginName = req.Username
	}
	if err := host.CompleteExternalLogin(ctx, w, r, req.ProviderID, entry.DN, loginName, entry.Email, entry.Name, entry.Groups); err != nil {
		p.host.Logger.Debug("identity-ldap: sign-in not completed", "provider_id", req.ProviderID, "error", err)
	}
}

// --- helpers ------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// errorResp is the host's { success, error, code } envelope: the login page renders
// `error` as text, so an object there would crash the form instead of showing it.
func errorResp(code, msg string) map[string]any {
	return map[string]any{
		"success": false,
		"error":   msg,
		"code":    code,
	}
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*IdentityLdapPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &IdentityLdapPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "identity-ldap",
		Name:    "LDAP / Active Directory",
		Version: "2026.10.01",
	},
}
