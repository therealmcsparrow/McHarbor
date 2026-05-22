// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers email server module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/email-servers", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersManage)).Post("/", h.HandleCreate)
			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersManage)).Put("/", h.HandleUpdate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersManage)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersManage)).Post("/default", h.HandleSetDefault)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEmailServersManage)).Post("/test", h.HandleTest)
			})
		})
	})
}
