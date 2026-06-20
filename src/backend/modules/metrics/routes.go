// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers metrics module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/metrics", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/host", h.HandleHostInfo)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/containers", h.HandleContainerStats)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/containers/{id}/stream", h.HandleContainerStatsStream)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostManage)).Post("/host/prune", h.HandleHostPrune)
		})
	})
}
