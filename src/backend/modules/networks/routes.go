// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers network module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/networks", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/", h.HandleCreate)

			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksView)).Get("/", h.HandleInspect)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksDelete)).Delete("/", h.HandleRemove)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/connect", h.HandleConnect)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/disconnect", h.HandleDisconnect)
			})
		})
	})
}
