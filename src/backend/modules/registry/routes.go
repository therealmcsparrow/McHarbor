// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package registry

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers registry management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/registries", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRegistryView)).Get("/", h.HandleList, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRegistryManage)).Post("/", h.HandleCreate, apispec.Request(CreateRegistryInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRegistryView)).Get("/{id}", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRegistryManage)).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateRegistryInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRegistryManage)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRegistryManage)).Post("/{id}/test", h.HandleTestConnection)
		})
	})
}
