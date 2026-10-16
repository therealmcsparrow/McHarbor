// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
)

// Failure reasons the host understands. Only reasonUnavailable is exempt from
// lockout, because a directory outage is nobody's wrong guess.
const (
	reasonInvalidCredentials = "invalid_credentials"
	reasonUnavailable        = "directory_unavailable"
)

// externalLoginHost is the host capability this plugin signs users in through.
// It is declared here, not in core/plugin, so adding it changed no shared type and
// broke no installed plugin: host.Identity is type-asserted to it at request time,
// and a host that predates it fails the assertion and the route answers 503.
//
// Every method except LDAPProviderConfig writes the response whenever it refuses,
// fails or completes; the plugin never shapes a login reply or touches a session.
type externalLoginHost interface {
	LDAPProviderConfig(ctx context.Context, providerID string) (map[string]string, error)
	BeginExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username string) error
	CheckExternalAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, email string) error
	FailExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, reason string)
	CompleteExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, externalID, username, email, displayName string, groups []string) error
}

// loginHost returns the host's external-login capability, or false on a host
// that predates it.
func (p *IdentityLdapPlugin) loginHost() (externalLoginHost, bool) {
	if p.host.Identity == nil {
		return nil, false
	}
	host, ok := p.host.Identity.(externalLoginHost)
	return host, ok
}

// directory returns the injected directory, or the real network one.
func (p *IdentityLdapPlugin) directory() ldapDirectory {
	if p.dir != nil {
		return p.dir
	}
	return netDirectory{}
}
