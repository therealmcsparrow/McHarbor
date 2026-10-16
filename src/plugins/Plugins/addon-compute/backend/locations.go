// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// globalLocationScope is the scope_id of the row that applies to every host.
//
// Scope ids are otherwise environment ids, so a reserved word cannot collide
// with one: an xid is 20 lowercase alphanumerics and never this.
const globalLocationScope = "global"

// ErrInvalidLocation refuses a path the agent would never accept.
var ErrInvalidLocation = errors.New("a compute location must be an absolute path")

// Locations is one stored row: where a scope wants its compute state kept.
//
// An empty field means "not set at this scope", which is what lets an
// environment override one location and inherit the rest.
type Locations struct {
	ID        string `json:"id,omitempty"`
	ScopeID   string `json:"scopeId"`
	VMPath    string `json:"vmPath,omitempty"`
	DiskPath  string `json:"diskPath,omitempty"`
	ImagePath string `json:"imagePath,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// EffectiveLocations is what a host actually uses, and where each value came
// from — the operator cannot otherwise tell an inherited path from a set one.
type EffectiveLocations struct {
	EnvironmentID string `json:"environmentId"`
	VMPath        string `json:"vmPath,omitempty"`
	DiskPath      string `json:"diskPath,omitempty"`
	ImagePath     string `json:"imagePath,omitempty"`
	VMSource      string `json:"vmSource"`
	DiskSource    string `json:"diskSource"`
	ImageSource   string `json:"imageSource"`
	// Applied is false when the agent did not take the configuration, which is
	// a different failure from a rejected request: the input was fine.
	Applied   bool   `json:"applied"`
	LastError string `json:"lastError,omitempty"`
}

// EnvironmentLocationsView is everything the host detail page's roots section
// needs in one read: what is stored here, what is in force, and what it would
// fall back to.
type EnvironmentLocationsView struct {
	Stored        Locations          `json:"stored"`
	Effective     EffectiveLocations `json:"effective"`
	Global        Locations          `json:"global"`
	AgentDefaults Locations          `json:"agentDefaults"`
}

// LocationsView lists every stored scope, for a settings surface that edits
// the global row and sees which hosts override it.
type LocationsView struct {
	Global    Locations   `json:"global"`
	Overrides []Locations `json:"overrides"`
}

// validLocationPath accepts an absolute POSIX path and nothing else.
//
// The agent is handed these verbatim, and a relative path there resolves
// against whatever directory the agent happens to run in.
func validLocationPath(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "\x00") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return false
		}
	}
	return len(value) <= 512
}

// Validate refuses a request the agent would only reject later, or silently
// resolve against its own working directory.
func (l Locations) Validate() error {
	for _, value := range []string{l.VMPath, l.DiskPath, l.ImagePath} {
		if !validLocationPath(strings.TrimSpace(value)) {
			return ErrInvalidLocation
		}
	}
	return nil
}

// locationRow reads one scope's row, answering a zero value when none exists.
func (s *Service) locationRow(ctx context.Context, scopeID string) (Locations, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, scope_id, COALESCE(vm_path, ''), COALESCE(disk_path, ''),
		       COALESCE(image_path, ''), created_at, updated_at
		  FROM plugin_addon_compute_locations
		 WHERE scope_id = ? LIMIT 1`, scopeID)
	if err != nil {
		return Locations{}, fmt.Errorf("reading compute locations for %s: %w", scopeID, err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Locations{}, fmt.Errorf("reading compute locations for %s: %w", scopeID, err)
		}
		return Locations{ScopeID: scopeID}, nil
	}

	var out Locations
	if err := rows.Scan(&out.ID, &out.ScopeID, &out.VMPath, &out.DiskPath,
		&out.ImagePath, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return Locations{}, fmt.Errorf("scanning compute locations for %s: %w", scopeID, err)
	}
	return out, rows.Err()
}

// agentDefaults reports the locations the agent uses when McHarbor names none.
//
// Read from the agent rather than hardcoded here: they are the agent's own
// build-time defaults and have moved once already.
func (s *Service) agentDefaults(ctx context.Context, envID string) Locations {
	if s.rpc == nil {
		return Locations{ScopeID: envID}
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// An empty RootsConfig changes nothing and answers with what is in force,
	// which is the only way to ask the agent what its defaults are.
	roots, err := s.rpc.ConfigureRoots(callCtx, envID, RootsConfig{})
	if err != nil {
		s.logger.Warn("compute: reading agent roots failed", "env", envID, "error", err)
		return Locations{ScopeID: envID}
	}
	return Locations{
		ScopeID:   envID,
		VMPath:    roots.VMRoot,
		DiskPath:  roots.CTRoot,
		ImagePath: roots.ImageRoot,
	}
}

// resolve picks the winning value per field and names where it came from.
func resolve(stored, global, defaults Locations, envID string) EffectiveLocations {
	pick := func(scoped, glob, fallback string) (string, string) {
		if scoped != "" {
			return scoped, "environment"
		}
		if glob != "" {
			return glob, "global"
		}
		return fallback, "agent"
	}

	out := EffectiveLocations{EnvironmentID: envID}
	out.VMPath, out.VMSource = pick(stored.VMPath, global.VMPath, defaults.VMPath)
	out.DiskPath, out.DiskSource = pick(stored.DiskPath, global.DiskPath, defaults.DiskPath)
	out.ImagePath, out.ImageSource = pick(stored.ImagePath, global.ImagePath, defaults.ImagePath)
	return out
}

// LocationsFor answers what one host keeps where, and what it inherited.
func (s *Service) LocationsFor(ctx context.Context, envID string) (EnvironmentLocationsView, error) {
	if envID == "" {
		return EnvironmentLocationsView{}, ErrNotFound
	}
	stored, err := s.locationRow(ctx, envID)
	if err != nil {
		return EnvironmentLocationsView{}, err
	}
	global, err := s.locationRow(ctx, globalLocationScope)
	if err != nil {
		return EnvironmentLocationsView{}, err
	}
	defaults := s.agentDefaults(ctx, envID)

	effective := resolve(stored, global, defaults, envID)
	effective.Applied = true

	return EnvironmentLocationsView{
		Stored:        stored,
		Effective:     effective,
		Global:        global,
		AgentDefaults: defaults,
	}, nil
}

// ListLocations reports the global row and every environment that overrides it.
func (s *Service) ListLocations(ctx context.Context) (LocationsView, error) {
	global, err := s.locationRow(ctx, globalLocationScope)
	if err != nil {
		return LocationsView{}, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, scope_id, COALESCE(vm_path, ''), COALESCE(disk_path, ''),
		       COALESCE(image_path, ''), created_at, updated_at
		  FROM plugin_addon_compute_locations
		 WHERE scope_id <> ?
		 ORDER BY scope_id LIMIT ?`, globalLocationScope, maxRows)
	if err != nil {
		return LocationsView{}, fmt.Errorf("listing compute locations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := LocationsView{Global: global, Overrides: []Locations{}}
	for rows.Next() {
		var entry Locations
		if err := rows.Scan(&entry.ID, &entry.ScopeID, &entry.VMPath, &entry.DiskPath,
			&entry.ImagePath, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
			return LocationsView{}, fmt.Errorf("scanning compute locations: %w", err)
		}
		out.Overrides = append(out.Overrides, entry)
	}
	return out, rows.Err()
}

// upsertLocations writes one scope's row, keeping the existing id so a second
// save does not orphan the first.
func (s *Service) upsertLocations(ctx context.Context, scopeID string, in Locations) error {
	now := time.Now().UTC().Format(time.RFC3339)
	existing, err := s.locationRow(ctx, scopeID)
	if err != nil {
		return err
	}

	if existing.ID == "" {
		_, err := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_compute_locations
			       (id, scope_id, vm_path, disk_path, image_path, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), scopeID, in.VMPath, in.DiskPath, in.ImagePath, now, now)
		if err != nil {
			return fmt.Errorf("storing compute locations for %s: %w", scopeID, err)
		}
		return nil
	}

	_, err = s.db.Exec(ctx, `
		UPDATE plugin_addon_compute_locations
		   SET vm_path = ?, disk_path = ?, image_path = ?, updated_at = ?
		 WHERE scope_id = ?`,
		in.VMPath, in.DiskPath, in.ImagePath, now, scopeID)
	if err != nil {
		return fmt.Errorf("updating compute locations for %s: %w", scopeID, err)
	}
	return nil
}

// SaveLocations stores one host's override and asks the agent to adopt it.
//
// The agent's answer is read rather than assumed: it declines a move that would
// strand a running guest, and the operator is told that instead of being shown
// a path nothing is using.
func (s *Service) SaveLocations(ctx context.Context, envID string, in Locations) (EnvironmentLocationsView, error) {
	if envID == "" {
		return EnvironmentLocationsView{}, ErrNotFound
	}
	in.VMPath = strings.TrimSpace(in.VMPath)
	in.DiskPath = strings.TrimSpace(in.DiskPath)
	in.ImagePath = strings.TrimSpace(in.ImagePath)
	if err := in.Validate(); err != nil {
		return EnvironmentLocationsView{}, err
	}
	if err := s.upsertLocations(ctx, envID, in); err != nil {
		return EnvironmentLocationsView{}, err
	}
	return s.applyLocations(ctx, envID)
}

// SaveGlobalLocations stores the McHarbor-wide default every host inherits
// until it overrides it.
func (s *Service) SaveGlobalLocations(ctx context.Context, in Locations) (LocationsView, error) {
	in.VMPath = strings.TrimSpace(in.VMPath)
	in.DiskPath = strings.TrimSpace(in.DiskPath)
	in.ImagePath = strings.TrimSpace(in.ImagePath)
	if err := in.Validate(); err != nil {
		return LocationsView{}, err
	}
	if err := s.upsertLocations(ctx, globalLocationScope, in); err != nil {
		return LocationsView{}, err
	}
	return s.ListLocations(ctx)
}

// ApplyLocations re-sends what is stored to a host that did not take it.
func (s *Service) ApplyLocations(ctx context.Context, envID string) (EnvironmentLocationsView, error) {
	if envID == "" {
		return EnvironmentLocationsView{}, ErrNotFound
	}
	return s.applyLocations(ctx, envID)
}

// RemoveLocations drops one host's override, returning it to what it inherits.
func (s *Service) RemoveLocations(ctx context.Context, envID string) (EnvironmentLocationsView, error) {
	if envID == "" || envID == globalLocationScope {
		return EnvironmentLocationsView{}, ErrNotFound
	}
	if _, err := s.db.Exec(ctx, `
		DELETE FROM plugin_addon_compute_locations WHERE scope_id = ?`, envID); err != nil {
		return EnvironmentLocationsView{}, fmt.Errorf("removing compute locations for %s: %w", envID, err)
	}
	return s.applyLocations(ctx, envID)
}

// applyLocations sends the resolved roots to the agent and reports what came
// back, so a refusal reaches the operator as a state rather than as silence.
func (s *Service) applyLocations(ctx context.Context, envID string) (EnvironmentLocationsView, error) {
	view, err := s.LocationsFor(ctx, envID)
	if err != nil {
		return EnvironmentLocationsView{}, err
	}
	if s.rpc == nil {
		view.Effective.Applied = false
		return view, nil
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	roots, err := s.rpc.ConfigureRoots(callCtx, envID, RootsConfig{
		VMRoot:    view.Effective.VMPath,
		CTRoot:    view.Effective.DiskPath,
		ImageRoot: view.Effective.ImagePath,
	})
	if err != nil {
		s.logger.Warn("compute: applying roots failed", "env", envID, "error", err)
		view.Effective.Applied = false
		view.Effective.LastError = hostErrorDetail(err)
		return view, nil
	}

	// What the host took, not what it was asked for.
	view.Effective.VMPath = roots.VMRoot
	view.Effective.DiskPath = roots.CTRoot
	view.Effective.ImagePath = roots.ImageRoot
	view.Effective.Applied = true
	view.Effective.LastError = ""

	s.recordAudit("plugin.compute.locations.applied", map[string]any{
		"environmentId": envID,
		"vmRoot":        roots.VMRoot,
		"ctRoot":        roots.CTRoot,
		"imageRoot":     roots.ImageRoot,
	})
	return view, nil
}
