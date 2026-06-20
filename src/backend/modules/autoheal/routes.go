// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers autoheal module routes.
func Mount(app *router.AppDeps) {
	svc := NewService(app.DockerPool, app.DB)
	h := NewHandler(app, svc)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/autoheal", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Get("/preference/{id}", h.HandleGetPreference)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/preference/{id}", h.HandleSetPreference)
		})
	})
}
