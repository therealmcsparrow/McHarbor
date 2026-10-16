// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package openapi

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/permcatalog"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Service reads the routes installed plugins contributed, which the recording
// router never sees because plugins register them at runtime.
type Service struct {
	db *sql.DB
}

// NewService returns a Service over the plugin route and permission tables.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// PluginOperations returns one operation per installed plugin route, carrying
// the permission and scope the plugin declared so the live document gates it too.
func (s *Service) PluginOperations(ctx context.Context) ([]apispec.Operation, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.origin, r.method, r.path, r.requires_permission, r.is_public,
		       COALESCE(p.scope, 'global')
		  FROM plugin_routes r
		  LEFT JOIN plugin_permissions p
		    ON p.origin = r.origin AND p.permission_key = r.requires_permission
		 ORDER BY r.origin, r.path, r.method
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("querying plugin routes: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("openapi: closing rows failed", "error", cerr)
		}
	}()

	var ops []apispec.Operation
	for rows.Next() {
		var origin, method, path, perm, scope string
		var public int
		if err := rows.Scan(&origin, &method, &path, &perm, &public, &scope); err != nil {
			return nil, fmt.Errorf("scanning plugin route: %w", err)
		}
		ops = append(ops, pluginOperation(origin, method, path, perm, scope, public == 1))
	}
	return ops, rows.Err()
}

func pluginOperation(origin, method, path, perm, scope string, public bool) apispec.Operation {
	if !strings.HasPrefix(path, "/api/") && path != "/api" {
		path = "/api" + ensureSlash(path)
	}
	op := apispec.Operation{
		Method:  strings.ToUpper(method),
		Path:    path,
		Group:   apispec.GroupProtected,
		Handler: "plugins/" + origin + ".Handle" + pathName(path),
		Module:  "plugin_" + origin,
		Tags:    []string{"plugin:" + origin},
		Summary: strings.ToUpper(method) + " " + path,
	}
	if public {
		op.Group = apispec.GroupPublic
	}
	if perm != "" {
		op.Perms = []rbac.PermRequirement{{Keys: []rbac.Permission{rbac.Permission(perm)}, Scope: rbac.PermScope(permcatalog.CatalogScope(scope))}}
	} else if op.Mutating() {
		op.Exempt = "plugin route declares no permission"
	}
	return op
}

func ensureSlash(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

// pathName turns /api/vms/{id}/start into VmsIdStart, so plugin operations get
// readable, stable operationIds without the plugin naming each one.
func pathName(path string) string {
	var b strings.Builder
	upper := true
	for _, r := range strings.TrimPrefix(path, "/api") {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
			upper = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
