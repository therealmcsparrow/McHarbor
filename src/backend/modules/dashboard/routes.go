// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers dashboard module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/dashboard", func(r *apispec.Router) {
			// dashboard.view is granted to every seeded role by migration 098.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermDashboardView)).Get("/stats", h.HandleStats, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermDashboardView)).Get("/summary", h.HandleSummary)
		})
	})
}
