// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package volumes

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers volume module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/volumes", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesManage)).Post("/", h.HandleCreate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesManage)).Post("/prune", h.HandlePrune)

			r.Route("/{name}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesView)).Get("/", h.HandleInspect)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermVolumesDelete)).Delete("/", h.HandleRemove)
			})
		})
	})
}
