// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package accessreview serves the security-review surfaces over RBAC: the
// effective permissions of one user with provenance, and the permission
// catalog export with the roles that hold each key.
package accessreview

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/permcatalog"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// ErrUserNotFound is returned when the reviewed user does not exist.
var ErrUserNotFound = errors.New("user not found")

// ErrEnvironmentNotFound is returned when the requested scope names an unknown environment.
var ErrEnvironmentNotFound = errors.New("environment not found")

// Service reads the permission catalog and resolves effective permissions
// through core/rbac; it holds no authorization logic of its own.
type Service struct {
	db     *sql.DB
	rbac   *rbac.Service
	logger *slog.Logger
}

// NewService builds the access review service.
func NewService(db *sql.DB, rbacSvc *rbac.Service, logger *slog.Logger) *Service {
	return &Service{db: db, rbac: rbacSvc, logger: logger}
}

// RoleHolder names a role that carries a catalog key; Wildcard marks a role
// that holds it only through "*".
type RoleHolder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Wildcard bool   `json:"wildcard,omitempty"`
}

// CatalogExportEntry is one permission in the export.
type CatalogExportEntry struct {
	Key         string       `json:"key"`
	Category    string       `json:"category"`
	Action      string       `json:"action"`
	Description string       `json:"description"`
	Scope       string       `json:"scope"`
	Origin      string       `json:"origin"`
	Elevatable  bool         `json:"elevatable"`
	Roles       []RoleHolder `json:"roles"`
}

// CatalogExport is the JSON export document.
type CatalogExport struct {
	GeneratedAt string               `json:"generatedAt"`
	Permissions []CatalogExportEntry `json:"permissions"`
	Roles       []ExportRole         `json:"roles"`
}

// ExportRole lists a role and how many catalog keys it holds.
type ExportRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Wildcard    bool   `json:"wildcard"`
	Permissions int    `json:"permissions"`
}

// EffectivePermissions is the effective-permission viewer's response: the
// rbac explanation plus display names for the ids it references.
type EffectivePermissions struct {
	*rbac.PermissionExplanation
	Username         string            `json:"username"`
	RoleNames        map[string]string `json:"roleNames"`
	GroupNames       map[string]string `json:"groupNames"`
	EnvironmentNames map[string]string `json:"environmentNames"`
}

// Export builds the catalog export: host and plugin keys, and for each the
// roles that grant it, wildcard roles included and flagged.
func (s *Service) Export(ctx context.Context) (*CatalogExport, error) {
	entries := permcatalog.Load(ctx, s.db, s.logger)
	hostKeys := make(map[string]bool)
	for _, e := range rbac.CatalogSeed() {
		hostKeys[string(e.Key)] = true
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, name, permissions FROM roles ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading roles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()
	type roleRow struct {
		id, name string
		keys     map[string]bool
		wildcard bool
	}
	var roles []roleRow
	for rows.Next() {
		var r roleRow
		var raw string
		if err := rows.Scan(&r.id, &r.name, &raw); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		r.keys = map[string]bool{}
		for _, key := range parsePermissionList(raw) {
			if key == string(rbac.PermWildcard) {
				r.wildcard = true
			}
			r.keys[key] = true
		}
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := &CatalogExport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Permissions: make([]CatalogExportEntry, 0, len(entries)),
		Roles:       make([]ExportRole, 0, len(roles)),
	}
	counts := make(map[string]int, len(roles))
	for _, e := range entries {
		origin := "host"
		if !hostKeys[e.Key] {
			origin = "plugin:" + e.Category
		}
		entry := CatalogExportEntry{
			Key: e.Key, Category: e.Category, Action: e.Action, Description: e.Description,
			Scope: e.Scope, Origin: origin, Elevatable: e.Elevatable, Roles: []RoleHolder{},
		}
		for _, r := range roles {
			switch {
			case r.keys[e.Key]:
				entry.Roles = append(entry.Roles, RoleHolder{ID: r.id, Name: r.name})
				counts[r.id]++
			case r.wildcard:
				entry.Roles = append(entry.Roles, RoleHolder{ID: r.id, Name: r.name, Wildcard: true})
				counts[r.id]++
			}
		}
		out.Permissions = append(out.Permissions, entry)
	}
	sort.Slice(out.Permissions, func(i, j int) bool { return out.Permissions[i].Key < out.Permissions[j].Key })
	for _, r := range roles {
		out.Roles = append(out.Roles, ExportRole{ID: r.id, Name: r.name, Wildcard: r.wildcard, Permissions: counts[r.id]})
	}
	return out, nil
}

// ExportCSV renders the export as CSV, one row per permission, with every
// cell guarded against spreadsheet formula injection.
func ExportCSV(export *CatalogExport) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"key", "category", "action", "description", "scope", "origin", "elevatable", "roles"}); err != nil {
		return nil, err
	}
	for _, p := range export.Permissions {
		names := make([]string, 0, len(p.Roles))
		for _, r := range p.Roles {
			if r.Wildcard {
				names = append(names, r.Name+" (*)")
			} else {
				names = append(names, r.Name)
			}
		}
		elevatable := "false"
		if p.Elevatable {
			elevatable = "true"
		}
		record := []string{p.Key, p.Category, p.Action, p.Description, p.Scope, p.Origin, elevatable, strings.Join(names, "; ")}
		for i := range record {
			record[i] = safeCSVCell(record[i])
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func safeCSVCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}

// Effective resolves one user's permissions at an environment/stack scope via
// rbac.ExplainPermissions, over the full catalog including plugin keys.
func (s *Service) Effective(ctx context.Context, userID, envID, stackName string) (*EffectivePermissions, error) {
	var username string
	err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}
	if envID != "" {
		var one int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM environments WHERE id = ?`, envID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEnvironmentNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("loading environment: %w", err)
		}
	}

	var keys []rbac.Permission
	for _, e := range permcatalog.Load(ctx, s.db, s.logger) {
		keys = append(keys, rbac.Permission(e.Key))
	}
	explanation, err := s.rbac.ExplainPermissions(ctx, userID, envID, stackName, keys)
	if err != nil {
		return nil, err
	}
	out := &EffectivePermissions{
		PermissionExplanation: explanation,
		Username:              username,
		RoleNames:             map[string]string{},
		GroupNames:            map[string]string{},
		EnvironmentNames:      map[string]string{},
	}
	for _, p := range explanation.Permissions {
		for _, src := range p.Sources {
			if src.RoleID != "" {
				out.RoleNames[src.RoleID] = src.RoleName
			}
			if src.GroupID != "" {
				out.GroupNames[src.GroupID] = src.GroupName
			}
			if src.EnvironmentID != "" {
				out.EnvironmentNames[src.EnvironmentID] = ""
			}
		}
	}
	if envID != "" {
		out.EnvironmentNames[envID] = ""
	}
	if err := s.fillEnvironmentNames(ctx, out.EnvironmentNames); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) fillEnvironmentNames(ctx context.Context, names map[string]string) error {
	if len(names) == 0 {
		return nil
	}
	ids := make([]any, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM environments WHERE id IN ("+placeholders+")", ids...) // SQL identifier: placeholders is only repeated "?," markers, every value is bound
	if err != nil {
		return fmt.Errorf("loading environment names: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		names[id] = name
	}
	return rows.Err()
}

func parsePermissionList(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
