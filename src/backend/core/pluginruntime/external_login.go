// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ExternalLoginProviderType is the only provider type whose credentials a plugin
// checks itself; OIDC and SAML redirect through the host's own dispatcher.
const ExternalLoginProviderType = "ldap"

// ErrExternalLoginNotOwner is returned to a plugin that calls the external-login
// surface without owning the ldap provider type, so no other plugin can read its
// directory credentials or mint a sign-in on its behalf.
var ErrExternalLoginNotOwner = errors.New("external login: plugin does not own the ldap provider type")

// ExternalLoginSource is the host half of a sign-in whose credential a plugin
// checks against an external directory. Every method except LDAPProviderConfig
// writes the HTTP response itself whenever it refuses, fails or completes, so the
// plugin never shapes a login response or holds a session.
//
// The signatures use only builtin and standard-library types on purpose: the
// plugin declares the same interface locally and type-asserts host.Identity to it,
// which keeps the capability out of core/plugin and so out of every plugin's ABI.
type ExternalLoginSource interface {
	// LDAPProviderConfig returns an enabled ldap provider's configuration with its
	// secrets decrypted; it is server-side only and never reaches a client.
	LDAPProviderConfig(ctx context.Context, providerID string) (map[string]string, error)
	// BeginExternalLogin refuses a locked client address before the directory is
	// contacted; a non-nil error means the refusal has been written to w.
	BeginExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username string) error
	// CheckExternalAccount refuses a locked or disabled linked account before the
	// user's own bind; a non-nil error means the refusal has been written to w.
	CheckExternalAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, email string) error
	// FailExternalLogin records a failed attempt against the address and any linked
	// account and writes the same refusal a failed password login gets.
	FailExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, reason string)
	// CompleteExternalLogin provisions or links the account per the provider's
	// settings, then finishes exactly as a password login does and writes the reply.
	CompleteExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, externalID, username, email, displayName string, groups []string) error
}

// externalLoginIdentityAdapter is the identity handle a plugin receives when the
// host wired external login. It is a distinct type from identityProvisionerAdapter
// so a host without the wiring fails the plugin's type assertion instead of
// handing it methods that do nothing.
type externalLoginIdentityAdapter struct {
	*identityProvisionerAdapter
	login   ExternalLoginSource
	slug    string
	runtime *Runtime
}

// identityHandleFor picks the identity handle for one plugin: nil without a
// provisioner, the plain adapter without external login, the extended one with it.
func (r *Runtime) identityHandleFor(slug string) plugin.IdentityProvisioner {
	if r.host.Identity == nil {
		return nil
	}
	base := &identityProvisionerAdapter{source: r.host.Identity}
	if r.host.ExternalLogin == nil {
		return base
	}
	return &externalLoginIdentityAdapter{identityProvisionerAdapter: base, login: r.host.ExternalLogin, slug: slug, runtime: r}
}

// ownsLDAP reports whether this adapter's plugin registered the ldap provider type.
// It is read at call time because registration order and reloads change the owner.
func (a *externalLoginIdentityAdapter) ownsLDAP() bool {
	if a.runtime == nil || a.runtime.adapters == nil {
		return false
	}
	return a.runtime.adapters.identityOwner(ExternalLoginProviderType) == a.slug
}

// writeNotOwner answers a call from a plugin that does not own the ldap type with a
// bare 403, because such a call is a plugin bug or abuse and never a user error.
func writeNotOwner(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

// refuseNotOwner writes the 403 and returns the sentinel the caller propagates.
func refuseNotOwner(w http.ResponseWriter) error {
	writeNotOwner(w)
	return ErrExternalLoginNotOwner
}

// LDAPProviderConfig forwards to the host source for the owning plugin only.
func (a *externalLoginIdentityAdapter) LDAPProviderConfig(ctx context.Context, providerID string) (map[string]string, error) {
	if !a.ownsLDAP() {
		return nil, ErrExternalLoginNotOwner
	}
	return a.login.LDAPProviderConfig(ctx, providerID)
}

// BeginExternalLogin forwards to the host source for the owning plugin only.
func (a *externalLoginIdentityAdapter) BeginExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username string) error {
	if !a.ownsLDAP() {
		return refuseNotOwner(w)
	}
	return a.login.BeginExternalLogin(ctx, w, r, providerID, username)
}

// CheckExternalAccount forwards to the host source for the owning plugin only.
func (a *externalLoginIdentityAdapter) CheckExternalAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, email string) error {
	if !a.ownsLDAP() {
		return refuseNotOwner(w)
	}
	return a.login.CheckExternalAccount(ctx, w, r, providerID, username, externalID, email)
}

// FailExternalLogin forwards to the host source for the owning plugin only.
func (a *externalLoginIdentityAdapter) FailExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, reason string) {
	if !a.ownsLDAP() {
		writeNotOwner(w)
		return
	}
	a.login.FailExternalLogin(ctx, w, r, providerID, username, externalID, reason)
}

// CompleteExternalLogin forwards to the host source for the owning plugin only;
// without the check any plugin could sign anyone in by naming an external id.
func (a *externalLoginIdentityAdapter) CompleteExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, externalID, username, email, displayName string, groups []string) error {
	if !a.ownsLDAP() {
		return refuseNotOwner(w)
	}
	return a.login.CompleteExternalLogin(ctx, w, r, providerID, externalID, username, email, displayName, groups)
}

// identityOwner returns the slug of the plugin that registered providerType, or ""
// when none did.
func (a *adapterRegistry) identityOwner(providerType string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.identityOrigin[providerType]
}
