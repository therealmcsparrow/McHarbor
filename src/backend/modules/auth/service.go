// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service handles auth-adjacent persistence concerns for the auth module.
type Service struct {
	db *sql.DB
}

// OIDCProvider is the minimal provider payload exposed on the public auth status endpoint.
type OIDCProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"providerType"`
}

// LocalEnvironmentResult describes the outcome of setup-time local environment detection.
type LocalEnvironmentResult struct {
	ID         string
	SocketPath string
	Runtime    string
	Created    bool
}

// NewService creates a new auth service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// EnabledOIDCProviders returns all enabled identity providers for the login screen.
func (s *Service) EnabledOIDCProviders() ([]OIDCProvider, error) {
	rows, err := s.db.Query(
		`SELECT id, name, provider_type FROM identity_providers WHERE enabled = 1 ORDER BY name LIMIT 100`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying enabled identity providers: %w", err)
	}
	defer rows.Close()

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

// EnsureLocalEnvironment auto-creates a local Docker environment during first-run setup.
func (s *Service) EnsureLocalEnvironment() (*LocalEnvironmentResult, error) {
	socketPath, runtime := docker.DetectAnySocket()
	if socketPath == "" {
		return nil, nil
	}

	var existingID string
	err := s.db.QueryRow(
		`SELECT id FROM environments WHERE socket_path = ? LIMIT 1`,
		socketPath,
	).Scan(&existingID)
	switch err {
	case nil:
		return &LocalEnvironmentResult{
			ID:         existingID,
			SocketPath: socketPath,
			Runtime:    runtime,
			Created:    false,
		}, nil
	case sql.ErrNoRows:
	default:
		return nil, fmt.Errorf("querying local environment: %w", err)
	}

	if _, err := s.db.Exec(`UPDATE environments SET is_default = 0 WHERE is_default = 1`); err != nil {
		return nil, fmt.Errorf("clearing default environments: %w", err)
	}

	connectionType := "socket"
	if runtime == "podman" {
		connectionType = "podman"
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT INTO environments (
			id, name, orchestrator_type, connection_type, socket_path, is_default, is_active, created_at, updated_at
		) VALUES (?, ?, 'docker', ?, ?, 1, 1, ?, ?)`,
		id, "Local", connectionType, socketPath, now, now,
	); err != nil {
		return nil, fmt.Errorf("inserting local environment: %w", err)
	}

	return &LocalEnvironmentResult{
		ID:         id,
		SocketPath: socketPath,
		Runtime:    runtime,
		Created:    true,
	}, nil
}
