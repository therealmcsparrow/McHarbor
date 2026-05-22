// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers communication channel module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/communication-channels", func(r chi.Router) {
			r.Get("/capabilities", h.HandleCapabilities)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Post("/", h.HandleCreate)
			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Put("/", h.HandleUpdate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Post("/default", h.HandleSetDefault)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Post("/test", h.HandleTest)
			})
		})
	})
}
