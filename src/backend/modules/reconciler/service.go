// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// DesiredState represents a desired state configuration stored in the database.
type DesiredState struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Kind          string       `json:"kind"`
	EnvID         string       `json:"envId"`
	StackName     string       `json:"stackName"`
	ContainerName string       `json:"containerName"`
	ImageRef      string       `json:"imageRef"`
	DesiredStatus string       `json:"desiredStatus"` // running, stopped, removed
	RestartPolicy string       `json:"restartPolicy"` // always, unless-stopped, on-failure, no
	Config        string       `json:"config"`        // JSON string of extra container config
	LastReconcile string       `json:"lastReconcile"`
	DriftDetected bool         `json:"driftDetected"`
	LastReport    *DriftReport `json:"lastReport,omitempty"`
	Mode          string       `json:"mode"`
	// SourceID names the Git source that owns this row; such a row is edited in
	// the repository, never through the API.
	SourceID       string `json:"sourceId,omitempty"`
	SourcePath     string `json:"sourcePath,omitempty"`
	SourceCompose  string `json:"-"`
	LastEnforcedAt string `json:"lastEnforcedAt,omitempty"`
	// Source names the plugin whose kind this is; SubjectID is what it watches and
	// Spec the pinned properties. Empty Source means a built-in Docker kind.
	Source    string            `json:"source,omitempty"`
	SubjectID string            `json:"subjectId,omitempty"`
	Spec      map[string]string `json:"spec,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// CreateDesiredStateInput is the request body for creating a desired state.
type CreateDesiredStateInput struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Kind          string `json:"kind"`
	StackName     string `json:"stackName"`
	EnvID         string `json:"envId"`
	ContainerName string `json:"containerName"`
	ImageRef      string `json:"imageRef"`
	DesiredStatus string `json:"desiredStatus"`
	RestartPolicy string `json:"restartPolicy"`
	Config        string `json:"config"`
	Mode          string `json:"mode"`
	// Source, SubjectID and Spec describe a plugin kind; see DesiredState.
	Source    string            `json:"source,omitempty"`
	SubjectID string            `json:"subjectId,omitempty"`
	Spec      map[string]string `json:"spec,omitempty"`
}

// UpdateDesiredStateInput is the request body for updating a desired state.
type UpdateDesiredStateInput struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	StackName     *string `json:"stackName"`
	EnvID         *string `json:"envId"`
	ContainerName *string `json:"containerName"`
	ImageRef      *string `json:"imageRef"`
	DesiredStatus *string `json:"desiredStatus"`
	RestartPolicy *string `json:"restartPolicy"`
	Config        *string `json:"config"`
	Mode          *string `json:"mode"`
	// Spec replaces a plugin kind's pinned properties when set.
	Spec *map[string]string `json:"spec,omitempty"`
}

// DriftReport describes the difference between desired and actual state.
type DriftReport struct {
	DesiredStateID string      `json:"desiredStateId"`
	HasDrift       bool        `json:"hasDrift"`
	Diffs          []DriftDiff `json:"diffs"`
	CheckedAt      string      `json:"checkedAt"`
}

// DriftDiff represents a single field difference.
type DriftDiff struct {
	Class       string `json:"class"`
	Field       string `json:"field"`
	Expected    string `json:"expected"`
	Actual      string `json:"actual"`
	FirstSeenAt string `json:"firstSeenAt,omitempty"`
}

// Service handles desired state database operations.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService creates a new reconciler service. enc encrypts each row's config and spec
// at rest; without it every write that carries either is refused.
func NewService(db *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: db, enc: enc}
}

// List returns desired states with pagination. With envID it keeps that
// environment's rows and rows with no environment, which predate the create
// dialog sending one and would otherwise vanish from every view.
func (s *Service) List(ctx context.Context, page, perPage int, envID string) ([]DesiredState, int64, error) {
	where, args := "", []any{}
	if envID != "" {
		where, args = " WHERE env_id = ? OR plugin_env = ? OR (env_id IS NULL AND plugin_env = '')", []any{envID, envID}
	}
	var total int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM desired_states"+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting desired states: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+desiredColumns+` FROM desired_states`+where+` ORDER BY name ASC LIMIT ? OFFSET ?`, // SQL identifier: where is one of two fixed literals above; every value goes through ?
		append(args, perPage, offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying desired states: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("reconciler: closing rows failed", "error", cerr)
		}
	}()

	var items []DesiredState
	for rows.Next() {
		ds, err := scanDesired(rows, s.open)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, ds)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading rows: %w", err)
	}

	if items == nil {
		items = []DesiredState{}
	}

	return items, total, nil
}

// ByID returns a single desired state.
func (s *Service) ByID(ctx context.Context, id string) (*DesiredState, error) {
	ds, err := scanDesired(s.db.QueryRowContext(ctx, `SELECT `+desiredColumns+` FROM desired_states WHERE id = ?`, id), s.open)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

// Create inserts a new desired state.
func (s *Service) Create(ctx context.Context, input CreateDesiredStateInput) (*DesiredState, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var envID interface{}
	if input.EnvID != "" {
		envID = input.EnvID
	}
	mode := input.Mode
	if mode == "" {
		mode = ModeObserve
	}

	specJSON, pluginEnv := "", ""
	if input.Source != "" {
		raw, err := json.Marshal(restoreSpec(input.Spec, nil))
		if err != nil {
			return nil, fmt.Errorf("encoding desired spec: %w", err)
		}
		specJSON, pluginEnv = string(raw), input.EnvID
		// A plugin's scope is not always an environment (a Kubernetes cluster is
		// not), and env_id references environments, so only a real one goes there.
		if input.EnvID != "" && !s.environmentExists(ctx, input.EnvID) {
			envID = nil
		}
	}
	config, err := restoreConfigEnv(input.Config, "")
	if err != nil {
		return nil, err
	}
	sealedConfig, err := s.seal(config)
	if err != nil {
		return nil, err
	}
	sealedSpec, err := s.seal(specJSON)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO desired_states (id, name, description, kind, env_id, stack_name, container_name, image_ref,
		 desired_status, restart_policy, config, mode, drift_detected, created_at, updated_at, source, subject_id, spec, plugin_env)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.Description, input.Kind, envID, input.StackName, input.ContainerName, input.ImageRef,
		input.DesiredStatus, input.RestartPolicy, sealedConfig, mode, now, now, input.Source, input.SubjectID, sealedSpec, pluginEnv,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting desired state: %w", err)
	}

	return s.ByID(ctx, id)
}

// Update modifies an existing desired state.
func (s *Service) Update(ctx context.Context, id string, input UpdateDesiredStateInput) (*DesiredState, error) {
	existing, err := s.ByID(ctx, id)
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
		if config, err = restoreConfigEnv(*input.Config, existing.Config); err != nil {
			return nil, err
		}
	}
	stackName := existing.StackName
	if input.StackName != nil {
		stackName = *input.StackName
	}
	mode := existing.Mode
	if input.Mode != nil {
		mode = *input.Mode
	}
	// An empty environment is stored as NULL: env_id references environments,
	// and '' would fail the foreign key on every edit of a row without one.
	var envValue any
	if envID != "" && (existing.Source == "" || s.environmentExists(ctx, envID)) {
		envValue = envID
	}
	spec := existing.Spec
	if input.Spec != nil {
		spec = restoreSpec(*input.Spec, existing.Spec)
	}
	specJSON := ""
	if existing.Source != "" {
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, fmt.Errorf("encoding desired spec: %w", err)
		}
		specJSON = string(raw)
	}
	sealedConfig, err := s.seal(config)
	if err != nil {
		return nil, err
	}
	sealedSpec, err := s.seal(specJSON)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE desired_states SET name = ?, description = ?, env_id = ?, stack_name = ?, container_name = ?,
		 image_ref = ?, desired_status = ?, restart_policy = ?, config = ?, mode = ?, updated_at = ?, spec = ?
		 WHERE id = ?`,
		name, desc, envValue, stackName, containerName, imageRef, desiredStatus, restartPolicy, sealedConfig, mode, now, sealedSpec, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating desired state: %w", err)
	}

	return s.ByID(ctx, id)
}

// Delete removes a desired state.
func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM desired_states WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting desired state: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("desired state not found")
	}
	return nil
}

// MarkReconciled updates the last_reconcile timestamp and drift flag.
func (s *Service) MarkReconciled(ctx context.Context, id string, driftDetected bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		"UPDATE desired_states SET last_reconcile = ?, drift_detected = ?, updated_at = ? WHERE id = ?",
		now, driftDetected, now, id,
	)
	return err
}

// ListAll returns every desired state for the background loop, capped because
// one pass must stay bounded however many rows accumulate.
func (s *Service) ListAll(ctx context.Context) ([]DesiredState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+desiredColumns+` FROM desired_states ORDER BY id LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("querying desired states: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("reconciler: closing rows failed", "error", cerr)
		}
	}()

	var items []DesiredState
	for rows.Next() {
		ds, err := scanDesired(rows, s.open)
		if err != nil {
			return nil, err
		}
		items = append(items, ds)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating desired states: %w", err)
	}
	return items, nil
}

// MarkObserved records a background check. Unlike MarkReconciled it leaves
// updated_at alone, because nothing about the desired state itself changed.
func (s *Service) MarkObserved(ctx context.Context, id string, driftDetected bool, report *DriftReport, at time.Time) error {
	var raw any
	if report != nil {
		b, err := json.Marshal(report)
		if err != nil {
			return fmt.Errorf("encoding drift report: %w", err)
		}
		raw = string(b)
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE desired_states SET last_reconcile = ?, drift_detected = ?, last_report = COALESCE(?, last_report) WHERE id = ?",
		at.UTC().Format(time.RFC3339), driftDetected, raw, id,
	)
	return err
}

// desiredColumns is the one column list every read of desired_states uses, so
// a new column cannot be read by one query and missed by another.
const desiredColumns = `id, name, description, kind, env_id, stack_name, container_name, image_ref,
	desired_status, restart_policy, config, last_reconcile, drift_detected, last_report, mode, source_id,
	source_path, source_compose, last_enforced_at, created_at, updated_at, source, subject_id, spec, plugin_env`

type rowScanner interface {
	Scan(dest ...any) error
}

// scanDesired reads one desired_states row; open decrypts the config and spec columns,
// which are ciphertext at rest.
func scanDesired(row rowScanner, open func(string) (string, error)) (DesiredState, error) {
	var ds DesiredState
	var desc, kind, envID, stackName, restartPolicy, config, lastReconcile, lastReport sql.NullString
	var mode, sourceID, sourcePath, sourceCompose, lastEnforced sql.NullString
	var pluginSource, subjectID, spec, pluginEnv sql.NullString
	var driftDetected sql.NullBool
	if err := row.Scan(&ds.ID, &ds.Name, &desc, &kind, &envID, &stackName, &ds.ContainerName, &ds.ImageRef,
		&ds.DesiredStatus, &restartPolicy, &config, &lastReconcile, &driftDetected, &lastReport,
		&mode, &sourceID, &sourcePath, &sourceCompose, &lastEnforced,
		&ds.CreatedAt, &ds.UpdatedAt, &pluginSource, &subjectID, &spec, &pluginEnv); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ds, err
		}
		return ds, fmt.Errorf("scanning desired state: %w", err)
	}
	ds.Description = desc.String
	ds.Kind = kind.String
	if ds.Kind == "" {
		ds.Kind = KindContainer
	}
	ds.EnvID = envID.String
	ds.StackName = stackName.String
	ds.RestartPolicy = restartPolicy.String
	plainConfig, err := open(config.String)
	if err != nil {
		return ds, fmt.Errorf("desired state %s config: %w", ds.ID, err)
	}
	ds.Config = plainConfig
	ds.LastReconcile = lastReconcile.String
	ds.DriftDetected = driftDetected.Bool
	ds.Mode = mode.String
	if ds.Mode == "" {
		ds.Mode = ModeObserve
	}
	ds.SourceID = sourceID.String
	ds.SourcePath = sourcePath.String
	ds.SourceCompose = sourceCompose.String
	ds.LastEnforcedAt = lastEnforced.String
	ds.Source, ds.SubjectID = pluginSource.String, subjectID.String
	if ds.EnvID == "" {
		ds.EnvID = pluginEnv.String
	}
	plainSpec, err := open(spec.String)
	if err != nil {
		return ds, fmt.Errorf("desired state %s spec: %w", ds.ID, err)
	}
	if plainSpec != "" {
		if err := json.Unmarshal([]byte(plainSpec), &ds.Spec); err != nil {
			slog.Warn("reconciler: desired state spec is not valid JSON", "id", ds.ID, "error", err)
		}
	}
	if lastReport.Valid && lastReport.String != "" {
		var r DriftReport
		if err := json.Unmarshal([]byte(lastReport.String), &r); err == nil {
			ds.LastReport = &r
		}
	}
	return ds, nil
}

// ReportRow is one confirmed drift, flattened with the watch it belongs to so a
// report can sort and filter across every desired state at once.
type ReportRow struct {
	DesiredStateID string `json:"desiredStateId"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	EnvID          string `json:"envId"`
	Class          string `json:"class"`
	Field          string `json:"field"`
	Expected       string `json:"expected"`
	Actual         string `json:"actual"`
	FirstSeenAt    string `json:"firstSeenAt"`
	CheckedAt      string `json:"checkedAt"`
}

// Report returns every diff of every desired state with confirmed drift, oldest
// first-seen first, so what has been wrong longest leads. Unconfirmed drift is
// left out for the same reason the loop debounces it.
func (s *Service) Report(ctx context.Context, envID, class string) ([]ReportRow, error) {
	states, err := s.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	rows := []ReportRow{}
	for _, ds := range states {
		if !ds.DriftDetected || ds.LastReport == nil {
			continue
		}
		if envID != "" && ds.EnvID != envID && ds.EnvID != "" {
			continue
		}
		for _, d := range ds.LastReport.Diffs {
			if class != "" && d.Class != class {
				continue
			}
			rows = append(rows, ReportRow{
				DesiredStateID: ds.ID, Name: ds.Name, Kind: ds.Kind, EnvID: ds.EnvID,
				Class: d.Class, Field: d.Field, Expected: d.Expected, Actual: d.Actual,
				FirstSeenAt: d.FirstSeenAt, CheckedAt: ds.LastReport.CheckedAt,
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].FirstSeenAt < rows[j].FirstSeenAt })
	return rows, nil
}

// SetMode changes only a desired state's mode, which is what the enforce route
// and a lowering of the mode touch; nothing else about the intent moves.
func (s *Service) SetMode(ctx context.Context, id, mode string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE desired_states SET mode = ?, updated_at = ? WHERE id = ?`,
		mode, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("setting mode: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkEnforced records when enforcement last acted on a row, which is what the
// per-row cooldown is measured from.
func (s *Service) MarkEnforced(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE desired_states SET last_enforced_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("recording enforcement: %w", err)
	}
	return nil
}

// environmentExists reports whether id names a McHarbor environment, which is what
// decides whether a plugin state's scope can be stored in the env_id foreign key.
func (s *Service) environmentExists(ctx context.Context, id string) bool {
	var one int
	return s.db.QueryRowContext(ctx, `SELECT 1 FROM environments WHERE id = ? LIMIT 1`, id).Scan(&one) == nil
}

// The single-row environment lookups behind rbac.PinEnvToResource: a watch, a
// proposal or a Git source is authorized against the environment it belongs to.
const (
	watchEnvQuery    = `SELECT COALESCE(env_id, '') FROM desired_states WHERE id = ?`
	proposalEnvQuery = `SELECT COALESCE(env_id, '') FROM reconcile_proposals WHERE id = ?`
	sourceEnvQuery   = `SELECT COALESCE(env_id, '') FROM desired_state_sources WHERE id = ?`
)

// WatchEnvironment reports the environment a desired-state watch belongs to.
func (s *Service) WatchEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, watchEnvQuery, id)
}

// ProposalEnvironment reports the environment a reconcile proposal belongs to.
func (s *Service) ProposalEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, proposalEnvQuery, id)
}

// SourceEnvironment reports the environment a desired-state source belongs to.
func (s *Service) SourceEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, sourceEnvQuery, id)
}
