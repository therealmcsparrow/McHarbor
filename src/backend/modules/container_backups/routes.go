// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// orphanSweepGracePeriod is the delay between McHarbor startup and
// the first orphan destination cleanup sweep. Long enough that the
// migration runner and connection-pool warmup have completed, short
// enough that an operator's first interaction with the UI sees a
// clean state.
const orphanSweepGracePeriod = 30 * time.Second

// Mount registers container backup routes and kicks off the
// startup-only orphan destination reconciliation sweep.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	go h.service.reconcileOrphansOnStartup(app.Logger)

	app.RegisterPublicRoutes(func(r chi.Router) {
		r.Get("/container-backups/internal/transfers/{transferId}", h.HandleRestoreTransfer)
		r.Put("/container-backups/internal/agent-archives/{transferId}", h.HandleAgentArchiveUpload)
		r.Get("/container-backups/internal/agent-archives/{transferId}", h.HandleAgentArchiveDownload)
	})

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/containers/{id}/backups", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/options", h.HandleOptions)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/run", h.HandleRunAdhoc)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/restore-upload", h.HandleRestoreUpload)
		})
		r.Route("/container-backups", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/", h.HandleListPlans)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/", h.HandleCreatePlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/runs", h.HandleListRuns)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/runs/{runId}/download", h.HandleDownloadRun)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Post("/runs/{runId}/restore-options", h.HandleRestoreOptions)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/runs/{runId}/cancel", h.HandleCancelRun)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Delete("/runs/{runId}", h.HandleDeleteRun)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/runs/{runId}/restore", h.HandleRestoreRun)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/runs/{runId}/destinations/{destinationId}/retry-upload", h.HandleRetryDestinationUpload)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Put("/{planId}", h.HandleUpdatePlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Delete("/{planId}", h.HandleDeletePlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/{planId}/run", h.HandleRunPlan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/admin/relink-all", h.HandleRelinkAllContainerLinks)
		})
	})
}
