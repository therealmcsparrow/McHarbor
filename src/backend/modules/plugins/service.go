// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Service handles plugin business logic and database operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new plugins service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns a paginated list of plugins and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Plugin, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM plugins").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting plugins: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, version, description, author, source, config, enabled, installed_at, updated_at
		 FROM plugins ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing plugins: %w", err)
	}
	defer rows.Close()

	var items []Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning plugin row: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating plugin rows: %w", err)
	}

	if items == nil {
		items = []Plugin{}
	}

	return items, total, nil
}

// ByID returns a single plugin by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*Plugin, error) {
	var p Plugin
	var version, desc, author, source, config sql.NullString
	var enabled sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, version, description, author, source, config, enabled, installed_at, updated_at
		 FROM plugins WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &version, &desc, &author, &source, &config, &enabled, &p.InstalledAt, &p.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting plugin %s: %w", id, err)
	}

	p.Version = version.String
	p.Description = desc.String
	p.Author = author.String
	p.Source = source.String
	p.Config = config.String
	p.Enabled = enabled.Bool

	return &p, nil
}

// Install creates a new plugin record.
func (s *Service) Install(ctx context.Context, input InstallPluginInput) (*Plugin, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugins (id, name, version, source, config, enabled, installed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		id, input.Name, input.Version, input.Source, input.Config, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting plugin: %w", err)
	}

	return &Plugin{
		ID:          id,
		Name:        input.Name,
		Version:     input.Version,
		Source:      input.Source,
		Config:      input.Config,
		Enabled:     true,
		InstalledAt: now,
		UpdatedAt:   now,
	}, nil
}

// Update applies partial updates to an existing plugin. Returns the updated
// plugin or nil if the ID was not found.
func (s *Service) Update(ctx context.Context, id string, input UpdatePluginInput) (*Plugin, error) {
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM plugins WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking plugin existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Config != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE plugins SET config = ?, updated_at = ? WHERE id = ?", *input.Config, now, id); err != nil {
			return nil, fmt.Errorf("updating plugin config: %w", err)
		}
	}
	if input.Version != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE plugins SET version = ?, updated_at = ? WHERE id = ?", *input.Version, now, id); err != nil {
			return nil, fmt.Errorf("updating plugin version: %w", err)
		}
	}

	return s.ByID(ctx, id)
}

// Uninstall removes a plugin by ID. Returns true if a row was deleted.
func (s *Service) Uninstall(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM plugins WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting plugin %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// Toggle flips the enabled state of a plugin. Returns the new enabled state,
// or an error. Returns sql.ErrNoRows if the plugin was not found.
func (s *Service) Toggle(ctx context.Context, id string) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx, "SELECT enabled FROM plugins WHERE id = ?", id).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("looking up plugin enabled state: %w", err)
	}

	newEnabled := !enabled
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, "UPDATE plugins SET enabled = ?, updated_at = ? WHERE id = ?", newEnabled, now, id); err != nil {
		return false, fmt.Errorf("toggling plugin %s: %w", id, err)
	}

	return newEnabled, nil
}

// scanPlugin scans a Plugin from a sql.Rows iterator.
func scanPlugin(rows *sql.Rows) (Plugin, error) {
	var p Plugin
	var version, desc, author, source, config sql.NullString
	var enabled sql.NullBool

	if err := rows.Scan(&p.ID, &p.Name, &version, &desc, &author, &source, &config,
		&enabled, &p.InstalledAt, &p.UpdatedAt); err != nil {
		return Plugin{}, err
	}

	p.Version = version.String
	p.Description = desc.String
	p.Author = author.String
	p.Source = source.String
	p.Config = config.String
	p.Enabled = enabled.Bool

	return p, nil
}
