// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// DesiredState represents a desired state configuration stored in the database.
type DesiredState struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	EnvID         string `json:"envId"`
	ContainerName string `json:"containerName"`
	ImageRef      string `json:"imageRef"`
	DesiredStatus string `json:"desiredStatus"` // running, stopped, removed
	RestartPolicy string `json:"restartPolicy"` // always, unless-stopped, on-failure, no
	Config        string `json:"config"`        // JSON string of extra container config
	LastReconcile string `json:"lastReconcile"`
	DriftDetected bool   `json:"driftDetected"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// CreateDesiredStateInput is the request body for creating a desired state.
type CreateDesiredStateInput struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	EnvID         string `json:"envId"`
	ContainerName string `json:"containerName"`
	ImageRef      string `json:"imageRef"`
	DesiredStatus string `json:"desiredStatus"`
	RestartPolicy string `json:"restartPolicy"`
	Config        string `json:"config"`
}

// UpdateDesiredStateInput is the request body for updating a desired state.
type UpdateDesiredStateInput struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	EnvID         *string `json:"envId"`
	ContainerName *string `json:"containerName"`
	ImageRef      *string `json:"imageRef"`
	DesiredStatus *string `json:"desiredStatus"`
	RestartPolicy *string `json:"restartPolicy"`
	Config        *string `json:"config"`
}

// DriftReport describes the difference between desired and actual state.
type DriftReport struct {
	DesiredStateID string       `json:"desiredStateId"`
	HasDrift       bool         `json:"hasDrift"`
	Diffs          []DriftDiff  `json:"diffs"`
	CheckedAt      string       `json:"checkedAt"`
}

// DriftDiff represents a single field difference.
type DriftDiff struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Service handles desired state database operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new reconciler service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns all desired states with pagination.
func (s *Service) List(page, perPage int) ([]DesiredState, int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM desired_states").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting desired states: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, name, description, env_id, container_name, image_ref, desired_status,
		        restart_policy, config, last_reconcile, drift_detected, created_at, updated_at
		 FROM desired_states ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying desired states: %w", err)
	}
	defer rows.Close()

	var items []DesiredState
	for rows.Next() {
		var ds DesiredState
		var desc, envID, restartPolicy, config, lastReconcile sql.NullString
		var driftDetected sql.NullBool
		if err := rows.Scan(&ds.ID, &ds.Name, &desc, &envID, &ds.ContainerName, &ds.ImageRef,
			&ds.DesiredStatus, &restartPolicy, &config, &lastReconcile, &driftDetected,
			&ds.CreatedAt, &ds.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning desired state: %w", err)
		}
		ds.Description = desc.String
		ds.EnvID = envID.String
		ds.RestartPolicy = restartPolicy.String
		ds.Config = config.String
		ds.LastReconcile = lastReconcile.String
		ds.DriftDetected = driftDetected.Bool
		items = append(items, ds)
	}

	if items == nil {
		items = []DesiredState{}
	}

	return items, total, nil
}

// ByID returns a single desired state.
func (s *Service) ByID(id string) (*DesiredState, error) {
	var ds DesiredState
	var desc, envID, restartPolicy, config, lastReconcile sql.NullString
	var driftDetected sql.NullBool

	err := s.db.QueryRow(
		`SELECT id, name, description, env_id, container_name, image_ref, desired_status,
		        restart_policy, config, last_reconcile, drift_detected, created_at, updated_at
		 FROM desired_states WHERE id = ?`, id,
	).Scan(&ds.ID, &ds.Name, &desc, &envID, &ds.ContainerName, &ds.ImageRef,
		&ds.DesiredStatus, &restartPolicy, &config, &lastReconcile, &driftDetected,
		&ds.CreatedAt, &ds.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying desired state: %w", err)
	}

	ds.Description = desc.String
	ds.EnvID = envID.String
	ds.RestartPolicy = restartPolicy.String
	ds.Config = config.String
	ds.LastReconcile = lastReconcile.String
	ds.DriftDetected = driftDetected.Bool

	return &ds, nil
}

// Create inserts a new desired state.
func (s *Service) Create(input CreateDesiredStateInput) (*DesiredState, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var envID interface{}
	if input.EnvID != "" {
		envID = input.EnvID
	}

	_, err := s.db.Exec(
		`INSERT INTO desired_states (id, name, description, env_id, container_name, image_ref,
		 desired_status, restart_policy, config, drift_detected, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, input.Name, input.Description, envID, input.ContainerName, input.ImageRef,
		input.DesiredStatus, input.RestartPolicy, input.Config, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting desired state: %w", err)
	}

	return s.ByID(id)
}

// Update modifies an existing desired state.
func (s *Service) Update(id string, input UpdateDesiredStateInput) (*DesiredState, error) {
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
	envID := existing.EnvID
	if input.EnvID != nil {
		envID = *input.EnvID
	}
	containerName := existing.ContainerName
	if input.ContainerName != nil {
		containerName = *input.ContainerName
	}
	imageRef := existing.ImageRef
	if input.ImageRef != nil {
		imageRef = *input.ImageRef
	}
	desiredStatus := existing.DesiredStatus
	if input.DesiredStatus != nil {
		desiredStatus = *input.DesiredStatus
	}
	restartPolicy := existing.RestartPolicy
	if input.RestartPolicy != nil {
		restartPolicy = *input.RestartPolicy
	}
	config := existing.Config
	if input.Config != nil {
		config = *input.Config
	}

	_, err = s.db.Exec(
		`UPDATE desired_states SET name = ?, description = ?, env_id = ?, container_name = ?,
		 image_ref = ?, desired_status = ?, restart_policy = ?, config = ?, updated_at = ?
		 WHERE id = ?`,
		name, desc, envID, containerName, imageRef, desiredStatus, restartPolicy, config, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating desired state: %w", err)
	}

	return s.ByID(id)
}

// Delete removes a desired state.
func (s *Service) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM desired_states WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting desired state: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("desired state not found")
	}
	return nil
}

// MarkReconciled updates the last_reconcile timestamp and drift flag.
func (s *Service) MarkReconciled(id string, driftDetected bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		"UPDATE desired_states SET last_reconcile = ?, drift_detected = ?, updated_at = ? WHERE id = ?",
		now, driftDetected, now, id,
	)
	return err
}

// BuildDriftReport creates a placeholder drift report (actual Docker comparison requires runtime checks).
func (s *Service) BuildDriftReport(ds *DesiredState) *DriftReport {
	report := &DriftReport{
		DesiredStateID: ds.ID,
		HasDrift:       ds.DriftDetected,
		Diffs:          []DriftDiff{},
		CheckedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	// Serialize config for placeholder
	if ds.Config != "" {
		var configMap map[string]any
		if err := json.Unmarshal([]byte(ds.Config), &configMap); err == nil {
			// Config is valid JSON — drift checks would compare with actual state
		}
	}

	return report
}
