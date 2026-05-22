// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package blueprints

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Blueprint represents a blueprint template stored in the database.
type Blueprint struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	ComposeYAML string  `json:"composeYaml"`
	EnvVars     string  `json:"envVars"`     // JSON string of default env vars
	Icon        string  `json:"icon"`
	Version     string  `json:"version"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// CreateBlueprintInput is the request body for creating a blueprint.
type CreateBlueprintInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	ComposeYAML string `json:"composeYaml"`
	EnvVars     string `json:"envVars"`
	Icon        string `json:"icon"`
	Version     string `json:"version"`
}

// UpdateBlueprintInput is the request body for updating a blueprint.
type UpdateBlueprintInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Category    *string `json:"category"`
	ComposeYAML *string `json:"composeYaml"`
	EnvVars     *string `json:"envVars"`
	Icon        *string `json:"icon"`
	Version     *string `json:"version"`
}

// DeployInput is the request body for deploying a blueprint.
type DeployInput struct {
	StackName string            `json:"stackName"`
	EnvID     string            `json:"envId"`
	EnvVars   map[string]string `json:"envVars"`
}

// Service handles blueprint database operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new blueprints service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns all blueprints with pagination.
func (s *Service) List(page, perPage int) ([]Blueprint, int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM blueprints").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting blueprints: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		"SELECT id, name, description, category, compose_yaml, env_vars, icon, version, created_at, updated_at FROM blueprints ORDER BY name ASC LIMIT ? OFFSET ?",
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying blueprints: %w", err)
	}
	defer rows.Close()

	var items []Blueprint
	for rows.Next() {
		var b Blueprint
		var desc, category, envVars, icon, version sql.NullString
		if err := rows.Scan(&b.ID, &b.Name, &desc, &category, &b.ComposeYAML, &envVars, &icon, &version, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning blueprint: %w", err)
		}
		b.Description = desc.String
		b.Category = category.String
		b.EnvVars = envVars.String
		b.Icon = icon.String
		b.Version = version.String
		items = append(items, b)
	}

	if items == nil {
		items = []Blueprint{}
	}

	return items, total, nil
}

// ByID returns a single blueprint by ID.
func (s *Service) ByID(id string) (*Blueprint, error) {
	var b Blueprint
	var desc, category, envVars, icon, version sql.NullString

	err := s.db.QueryRow(
		"SELECT id, name, description, category, compose_yaml, env_vars, icon, version, created_at, updated_at FROM blueprints WHERE id = ?",
		id,
	).Scan(&b.ID, &b.Name, &desc, &category, &b.ComposeYAML, &envVars, &icon, &version, &b.CreatedAt, &b.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying blueprint: %w", err)
	}

	b.Description = desc.String
	b.Category = category.String
	b.EnvVars = envVars.String
	b.Icon = icon.String
	b.Version = version.String

	return &b, nil
}

// Create inserts a new blueprint.
func (s *Service) Create(input CreateBlueprintInput) (*Blueprint, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(
		"INSERT INTO blueprints (id, name, description, category, compose_yaml, env_vars, icon, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, input.Name, input.Description, input.Category, input.ComposeYAML, input.EnvVars, input.Icon, input.Version, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting blueprint: %w", err)
	}

	return s.ByID(id)
}

// Update modifies an existing blueprint.
func (s *Service) Update(id string, input UpdateBlueprintInput) (*Blueprint, error) {
	existing, err := s.ByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}
	desc := existing.Description
	if input.Description != nil {
		desc = *input.Description
	}
	category := existing.Category
	if input.Category != nil {
		category = *input.Category
	}
	composeYAML := existing.ComposeYAML
	if input.ComposeYAML != nil {
		composeYAML = *input.ComposeYAML
	}
	envVars := existing.EnvVars
	if input.EnvVars != nil {
		envVars = *input.EnvVars
	}
	icon := existing.Icon
	if input.Icon != nil {
		icon = *input.Icon
	}
	version := existing.Version
	if input.Version != nil {
		version = *input.Version
	}

	_, err = s.db.Exec(
		"UPDATE blueprints SET name = ?, description = ?, category = ?, compose_yaml = ?, env_vars = ?, icon = ?, version = ?, updated_at = ? WHERE id = ?",
		name, desc, category, composeYAML, envVars, icon, version, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating blueprint: %w", err)
	}

	return s.ByID(id)
}

// Delete removes a blueprint by ID.
func (s *Service) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM blueprints WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting blueprint: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("blueprint not found")
	}
	return nil
}
