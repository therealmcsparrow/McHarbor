// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers autoheal module routes.
func Mount(app *router.AppDeps) {
	svc := NewService(app.DockerPool, app.DB)
	h := NewHandler(app, svc)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/autoheal", func(r *apispec.Router) {
			// containers.manage keeps unlocking autoheal (historical gate);
			// the dedicated autoheal.* keys unlock it on their own.
			r.With(rbac.RequireAnyPermission(app.RBACService, rbac.PermContainersManage, rbac.PermAutohealView)).Get("/preference/{id}", h.HandleGetPreference, apispec.Scoped())
			r.With(rbac.RequireAnyPermission(app.RBACService, rbac.PermContainersManage, rbac.PermAutohealManage)).Post("/preference/{id}", h.HandleSetPreference, apispec.Scoped(), apispec.Request(PreferenceRequest{}))
		})
	})
}
