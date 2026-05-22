// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package groups

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers group management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/groups", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Post("/", h.HandleCreate)
			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Put("/", h.HandleUpdate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Post("/members", h.HandleAddMember)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Delete("/members/{userId}", h.HandleRemoveMember)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Post("/roles", h.HandleAssignRole)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGroupsManage)).Delete("/roles/{assignmentId}", h.HandleUnassignRole)
			})
		})
	})
}
