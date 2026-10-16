// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers activity module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/activity", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermActivityView)).Get("/", h.HandleList, apispec.Scoped(), apispec.Paginated(), apispec.Query("from", ""), apispec.Query("search", ""), apispec.Query("search_mode", ""), apispec.Query("to", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/", h.HandleCreate, apispec.Request(CreateRequest{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge, apispec.Query("vacuum", ""))
		})
	})
}
