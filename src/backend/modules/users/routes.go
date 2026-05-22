// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers user management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/users", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/", h.HandleList)
			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Put("/", h.HandleUpdate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Delete("/", h.HandleDelete)
				r.Put("/password", h.HandleChangePassword)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/groups", h.HandleListGroups)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/roles", h.HandleListRoles)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Post("/roles", h.HandleAssignRole)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Delete("/roles/{assignmentId}", h.HandleUnassignRole)
			})
		})
	})
}
