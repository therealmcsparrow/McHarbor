// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

import (
	"encoding/json"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers role management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/roles", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/", h.HandleList, apispec.Response([]Role{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/permissions", h.HandleListPermissions)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/{id}", h.HandleGet, apispec.Response(Role{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Post("/", h.HandleCreate, apispec.Request(CreateRoleInput{}), apispec.Response(Role{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateRoleInput{}), apispec.Response(Role{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Delete("/{id}", h.HandleDelete)

			// Phase 7 of the RBAC roadmap: permission impact preview.
			// Computes how many users would gain / lose access if we
			// saved the role with the supplied permission set. Used by
			// the role editor to show "N users would lose / gain" before
			// the admin clicks Save.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Post("/{id}/impact", h.HandleImpact, apispec.Request(ImpactInput{}))

			// Phase 5 of the RBAC roadmap: predefined permission sets
			// (Backup Manager, Read-Only Auditor, etc.) that admins can
			// use as a starting point for new custom roles. Read-only
			// from the client's perspective; presets are server-side
			// seed.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/presets", h.HandleListPresets)
		})
	})
}

// RolePreset is one row of the role_presets table exposed to the UI.
type RolePreset struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// HandleListPresets returns every preset from the role_presets table.
// Phase 5 of the RBAC roadmap.
func (h *Handler) HandleListPresets(w http.ResponseWriter, r *http.Request) {
	rows, err := h.app.DB.QueryContext(r.Context(),
		"SELECT key, name, description, permissions FROM role_presets ORDER BY name ASC LIMIT 1000",
	)
	if err != nil {
		h.app.Logger.Error("roles: list presets", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			h.app.Logger.Error("roles: closing preset rows failed", "error", cerr)
		}
	}()

	var items []RolePreset
	for rows.Next() {
		var p RolePreset
		var permsJSON string
		if err := rows.Scan(&p.Key, &p.Name, &p.Description, &permsJSON); err != nil {
			h.app.Logger.Error("roles: scan preset", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		// The permissions column is a JSON array. Tolerate malformed
		// rows by falling back to an empty slice rather than 500ing
		// the whole endpoint.
		if permsJSON != "" {
			if err := json.Unmarshal([]byte(permsJSON), &p.Permissions); err != nil {
				h.app.Logger.Warn("roles: preset permissions not valid JSON, using empty",
					"key", p.Key, "error", err)
				p.Permissions = []string{}
			}
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		h.app.Logger.Error("roles: iterate presets", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if items == nil {
		items = []RolePreset{}
	}
	response.OK(w, map[string]any{"presets": items})
}
