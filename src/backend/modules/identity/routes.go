// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers identity provider module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Public routes (no auth middleware) — for OIDC login flow
	app.RegisterAuthRoutes(func(r chi.Router) {
		r.Get("/identity-providers/enabled", h.HandleEnabledProviders)
		r.Get("/identity-providers/{id}/authorize", h.HandleAuthorize)
		r.Get("/identity-providers/callback", h.HandleCallback)
	})

	// Protected routes — admin CRUD
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/identity-providers", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Get("/{id}", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/", h.HandleCreate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/{id}", h.HandleUpdate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/{id}/test", h.HandleTest)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Get("/{id}/groups", h.HandleFetchGroups)
		})
	})
}
