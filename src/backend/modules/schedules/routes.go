// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package schedules

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers schedule management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/schedules", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSchedulesView)).Get("/", h.HandleList, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSchedulesManage)).Post("/", h.HandleCreate, apispec.Request(CreateScheduleInput{}), apispec.Response(Schedule{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSchedulesView)).Get("/{id}", h.HandleGet, apispec.Response(Schedule{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSchedulesManage)).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateScheduleInput{}), apispec.Response(Schedule{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSchedulesDelete)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSchedulesView)).Get("/{id}/executions", h.HandleListExecutions, apispec.Paginated())
		})
	})
}
