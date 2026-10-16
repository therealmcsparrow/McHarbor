// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers version routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/versions", func(r *apispec.Router) {
			r.Get("/", h.HandleInfo)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSystemManage)).Post("/self-update", h.HandleSelfUpdate, apispec.Request(SelfUpdateRequest{}))
		})
	})
}
