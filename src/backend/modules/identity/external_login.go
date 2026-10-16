// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package identity manages external identity providers, sign-in flows and group mapping.
package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// errProviderUnavailable covers a provider that is missing, disabled or of another
// type; the caller answers all three alike so a login form cannot probe which.
var errProviderUnavailable = errors.New("identity: provider unavailable")

// service builds the Service the host callbacks run on, with the vault wired so
// an adopted client secret decrypts the same way the admin surface reads it.
func (h *HostProvisioner) service() *Service {
	return NewService(h.db, h.encryption, h.authSvc, nil)
}

// ExternalProviderConfig returns an enabled provider of providerType with its
// secrets decrypted, for a plugin that checks credentials itself. The result is
// server-side only and must never be written to a response.
func (h *HostProvisioner) ExternalProviderConfig(ctx context.Context, providerID, providerType string) (map[string]string, error) {
	svc := h.service()
	provider, err := svc.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errProviderUnavailable, err)
	}
	if provider.ProviderType != providerType || !provider.Enabled {
		return nil, errProviderUnavailable
	}
	return svc.providerToConfigMap(ctx, provider)
}

// LinkedUserID returns the account a sign-in by externalID would land on — the
// row linked to it, else the row an email match would link — or "" when none.
// It mirrors findOrProvisionUser's lookup order so a lockout check and the login
// it guards agree on the account.
func (h *HostProvisioner) LinkedUserID(ctx context.Context, providerID, externalID, email string) (string, error) {
	if externalID == "" && email == "" {
		return "", nil
	}
	var userID string
	if externalID != "" {
		err := h.db.QueryRowContext(ctx,
			`SELECT id FROM users WHERE external_id = ? AND identity_provider_id = ?`,
			externalID, providerID,
		).Scan(&userID)
		if err == nil {
			return userID, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("querying linked user: %w", err)
		}
	}
	if email == "" {
		return "", nil
	}
	err := h.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying user by email: %w", err)
	}
	return userID, nil
}

// ProvisionExternalUser links or creates the account for a directory sign-in
// under the provider's own settings — auto-provisioning, default role, group
// claims, auto-import and mappings — as the OIDC and SAML callbacks do. It
// returns "" with a nil error when auto-provisioning is off and nothing matched.
func (h *HostProvisioner) ProvisionExternalUser(ctx context.Context, providerID, externalID, username, email, displayName string, groups []string) (string, error) {
	if externalID == "" {
		return "", errors.New("identity: external id is required")
	}
	svc := h.service()
	provider, err := svc.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errProviderUnavailable, err)
	}
	if !provider.Enabled {
		return "", errProviderUnavailable
	}
	user, err := svc.findOrProvisionUser(ctx, providerID, externalUserInfo{
		Subject:           externalID,
		Email:             email,
		Name:              displayName,
		Groups:            groups,
		UsernamePrefix:    svc.usernamePrefix(provider.ProviderType),
		PreferredUsername: username,
	}, provider.AutoProvision, provider.DefaultRoleID)
	if errors.Is(err, errUserNotProvisioned) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("provisioning external user: %w", err)
	}
	svc.syncProviderGroups(ctx, provider, user.ID, groups)
	return user.ID, nil
}

// freeUsername returns candidate normalised, or with a short unique suffix when a
// local or other-provider account already holds it, so provisioning a directory
// user never fails on a name collision.
func (s *Service) freeUsername(ctx context.Context, candidate string) (string, error) {
	candidate = auth.NormalizeUsername(candidate)
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ?`, candidate).Scan(&count); err != nil {
		return "", fmt.Errorf("checking username: %w", err)
	}
	if count == 0 {
		return candidate, nil
	}
	suffix := xid.New().String()
	return candidate + "_" + suffix[len(suffix)-6:], nil
}
