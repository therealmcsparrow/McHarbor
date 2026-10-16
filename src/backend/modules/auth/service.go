// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/therealmcsparrow/mcharbor/core/permcatalog"
)

// Service handles auth-adjacent persistence concerns for the auth module.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// OIDCProvider is the minimal provider payload exposed on the public auth status endpoint.
type OIDCProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"providerType"`
}

// PermissionCatalogEntry is the permission metadata exposed to the frontend; it
// is core/permcatalog's Entry, shared with access reviews.
type PermissionCatalogEntry = permcatalog.Entry

// NewService creates a new auth service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// PermissionCatalog returns the canonical permission metadata used by role
// editors: the host's own permissions plus the ones installed plugins declared.
func (s *Service) PermissionCatalog(ctx context.Context) []PermissionCatalogEntry {
	return permcatalog.Load(ctx, s.db, s.logger)
}

// EnabledOIDCProviders returns all enabled identity providers for the login screen.
func (s *Service) EnabledOIDCProviders(ctx context.Context) ([]OIDCProvider, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, provider_type FROM identity_providers WHERE enabled = 1 ORDER BY name LIMIT 100`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying enabled identity providers: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing identity provider rows failed", "error", err)
		}
	}()

	var providers []OIDCProvider
	for rows.Next() {
		var provider OIDCProvider
		if err := rows.Scan(&provider.ID, &provider.Name, &provider.ProviderType); err != nil {
			return nil, fmt.Errorf("scanning enabled identity provider: %w", err)
		}
		providers = append(providers, provider)
	}
	if providers == nil {
		providers = []OIDCProvider{}
	}

	return providers, rows.Err()
}
