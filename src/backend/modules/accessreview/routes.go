// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package accessreview

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers /api/access-review. Both routes read who-can-do-what, which
// is exactly what permissions.view ("read the permission catalog and effective
// permissions") covers, and both are global: the scope is a query value, not
// the caller's environment.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/access-review", func(r *apispec.Router) {
			view := rbac.RequireGlobalPermission(app.RBACService, rbac.PermPermissionsView)
			r.With(view).Get("/effective-permissions", h.HandleEffective,
				apispec.Query("userId", ""), apispec.Query("environmentId", ""), apispec.Query("stack", ""),
				apispec.Response(EffectivePermissions{}))
			r.With(view).Get("/permission-catalog/export", h.HandleCatalogExport, apispec.Query("format", ""))
		})
	})
}
