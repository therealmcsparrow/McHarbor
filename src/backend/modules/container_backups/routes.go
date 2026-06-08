// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers container backup routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/containers/{id}/backups", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/options", h.HandleOptions)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/run", h.HandleRunAdhoc)
		})
		r.Route("/container-backups", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/", h.HandleListPlans)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/", h.HandleCreatePlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/runs", h.HandleListRuns)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/runs/{runId}/download", h.HandleDownloadRun)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/runs/{runId}/restore", h.HandleRestoreRun)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Put("/{planId}", h.HandleUpdatePlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Delete("/{planId}", h.HandleDeletePlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/{planId}/run", h.HandleRunPlan)
		})
	})
}
