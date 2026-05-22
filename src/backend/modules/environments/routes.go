// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers environment module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/environments", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/", h.HandleCreate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/detect-socket", h.HandleDetectSocket)

			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Put("/", h.HandleUpdate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/test", h.HandleTestConnection)
			})
		})
	})
}
