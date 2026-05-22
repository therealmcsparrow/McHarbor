// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pods

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers pod routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/pods", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermPodsView)).Get("/", h.HandleList)
			r.Route("/{namespace}/{name}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermPodsView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermPodsDelete)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermPodsView)).Get("/logs", h.HandleLogs)
			})
		})
	})
}
