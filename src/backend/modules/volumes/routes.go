// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package volumes

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers volume module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/volumes", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesView)).Get("/", h.HandleList, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesManage)).Post("/", h.HandleCreate, apispec.Scoped(), apispec.Request(CreateRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesManage)).Post("/prune", h.HandlePrune, apispec.Scoped())

			r.Route("/{name}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesView)).Get("/", h.HandleInspect, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesDelete)).Delete("/", h.HandleRemove, apispec.Scoped(),
					apispec.Query("force", "Remove the volume even when it is in use."))
			})
		})
	})
}
