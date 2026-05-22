// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers role management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/roles", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/permissions", h.HandleListPermissions)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesView)).Get("/{id}", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Post("/", h.HandleCreate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Put("/{id}", h.HandleUpdate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRolesManage)).Delete("/{id}", h.HandleDelete)
		})
	})
}
