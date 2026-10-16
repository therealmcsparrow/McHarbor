// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package events

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers Docker events streaming module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/events", func(r *apispec.Router) {
			// events.view is granted to every seeded role by migration 098.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEventsView)).Get("/stream", h.HandleStream, apispec.SSE(), apispec.Scoped(), apispec.Query("type", ""))
		})
	})
}
