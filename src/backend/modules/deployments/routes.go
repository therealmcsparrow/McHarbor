// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deployments

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers deployment routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/deployments", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermDeploymentsView)).Get("/", h.HandleList)
			r.Route("/{namespace}/{name}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermDeploymentsView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermDeploymentsDelete)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermDeploymentsManage)).Post("/scale", h.HandleScale)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermDeploymentsManage)).Post("/restart", h.HandleRestart)
			})
		})
	})
}
