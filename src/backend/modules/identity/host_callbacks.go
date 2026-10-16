// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

// ErrUserNotFound is returned when auto-provisioning is disabled and
// no matching user exists.
var ErrUserNotFound = errors.New("identity: user not found and auto-provisioning disabled")

// HostProvisioner is the host-side implementation of the plugin SDK's
// IdentityProvisionerSource. It wraps the existing identity.Service
// user provisioning logic so identity plugins (OIDC, LDAP, ...) can
// authenticate external users without touching core `users` /
// `groups` / `group_members` / `user_roles` tables directly.
//
// This type is wired into core/plugin.Boot from main.go.
type HostProvisioner struct {
	db         *sql.DB
	encryption *encryption.Service
	authSvc    *auth.Service
}

// NewHostProvisioner constructs a HostProvisioner.
func NewHostProvisioner(db *sql.DB, enc *encryption.Service, authSvc *auth.Service) *HostProvisioner {
	return &HostProvisioner{db: db, encryption: enc, authSvc: authSvc}
}

// ProvisionUser implements pluginruntime.IdentityProvisionerSource.
// Takes the loader's IdentityProvisionRequest type so the interface is
// satisfied without an adapter (the loader's identityProvisionerAdapter
// already bridges plugin.SDK types to this loader-side type).
func (h *HostProvisioner) ProvisionUser(ctx context.Context, req pluginruntime.IdentityProvisionRequest) (*pluginruntime.IdentityProvisionedUser, error) {
	svc := &Service{db: h.db, encryption: h.encryption, authSvc: h.authSvc}
	// The plugin chooses the username prefix for the accounts it
	// provisions; the host only supplies a generic fallback so it never
	// has to know which identity plugins exist.
	prefix := strings.TrimSpace(req.UsernamePrefix)
	if prefix == "" {
		prefix = "idp_"
	}

	defaultRoleID := sql.NullString{}
	if req.DefaultRoleID != "" {
		defaultRoleID.String = req.DefaultRoleID
		defaultRoleID.Valid = true
	}

	user, err := svc.findOrProvisionUser(ctx, req.ProviderID, externalUserInfo{
		Subject:        req.Subject,
		Email:          req.Email,
		Name:           req.DisplayName,
		Groups:         req.Groups,
		UsernamePrefix: prefix,
	}, req.AutoProvision, defaultRoleID)
	if err != nil {
		if errors.Is(err, errUserNotProvisioned) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("identity provision: %w", err)
	}
	out := &pluginruntime.IdentityProvisionedUser{
		ID:       user.ID,
		Username: user.Username,
	}
	if user.DisplayName != nil {
		out.DisplayName = *user.DisplayName
	}
	if user.Email != nil {
		out.Email = *user.Email
	}
	return out, nil
}

// errUserNotProvisioned is the internal sentinel the service returns
// when auto-provisioning is disabled and no matching user exists.
// We re-export it as ErrUserNotFound to the plugin SDK.
var errUserNotProvisioned = errors.New("identity: user not found and auto-provisioning disabled")

// HostSessionIssuer is the host-side implementation of the plugin SDK's
// SessionIssuerSource. It wraps auth.Service.CreateSession plus the
// session cookie config so identity plugins can mint sessions without
// touching the core `sessions` table directly.
//
// This type is wired into core/plugin.Boot from main.go.
type HostSessionIssuer struct {
	authSvc *auth.Service
}

// NewHostSessionIssuer constructs a HostSessionIssuer.
func NewHostSessionIssuer(authSvc *auth.Service) *HostSessionIssuer {
	return &HostSessionIssuer{authSvc: authSvc}
}

// IssueSession implements pluginruntime.SessionIssuerSource.IssueSession.
// Returns the new session ID plus its lifetime in seconds (matches
// auth.SessionDuration so the cookie's Max-Age matches the DB row's
// expires_at).
func (h *HostSessionIssuer) IssueSession(userID string) (string, int, error) {
	sessionID, err := h.authSvc.CreateSession(userID)
	if err != nil {
		return "", 0, fmt.Errorf("identity issue session: %w", err)
	}
	return sessionID, int(auth.SessionDuration.Seconds()), nil
}

// SessionCookieName implements pluginruntime.SessionIssuerSource.SessionCookieName.
func (h *HostSessionIssuer) SessionCookieName() string {
	return auth.SessionCookie
}

// SessionIsSecure implements pluginruntime.SessionIssuerSource.SessionIsSecure.
// Returns true when the request arrived over TLS — either directly
// (r.TLS != nil) or via a reverse proxy that set X-Forwarded-Proto: https.
// Plugins use this to set the Secure flag on their Set-Cookie header.
func (h *HostSessionIssuer) SessionIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	return httpx.RequestScheme(r) == "https"
}
