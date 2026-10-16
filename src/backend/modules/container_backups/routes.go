// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// orphanSweepGracePeriod is the delay between McHarbor startup and
// the first orphan destination cleanup sweep. Long enough that the
// migration runner and connection-pool warmup have completed, short
// enough that an operator's first interaction with the UI sees a
// clean state.
const orphanSweepGracePeriod = 30 * time.Second

// Mount registers container backup routes with no plugin runtime attached.
//
// A host mounted this way serves the built-in destination types only:
// plugin-owned ones (google_drive, sharepoint, onedrive_*) fall through to
// the host's switch and report that the plugin is not installed, which is the
// correct behaviour for a build with the plugin runtime disabled. Production
// wiring uses MountWithRuntime.
func Mount(app *router.AppDeps) {
	MountWithRuntime(app, nil, nil)
}

// MountWithRuntime registers container backup routes and injects
// the loaded plugin runtime into the dispatcher's upload path.
// Without this, plugin-owned storage destinations return "storage
// location type X is not supported for container backup upload —
// install the matching plugin" even when the plugin is loaded.
//
// The boot ordering is: bootPluginRuntime → MountWithRuntime. The
// runtime is also pushed into the scheduler's service at the
// caller so both the scheduler and the HTTP route resolver see
// the same adapter map.
func MountWithRuntime(app *router.AppDeps, runtime any, recreator ContainerRecreator) {
	h := newMountedHandler(app, runtime, recreator)

	go h.service.reconcileOrphansOnStartup(app.Logger)

	// Runs after the runtime is injected, because a plugin driver's schema is the only
	// record of which snapshot config fields are secret. Idempotent; a failure is
	// logged and tolerated so one bad row cannot stop the server from starting.
	backfillCtx, cancelBackfill := context.WithTimeout(context.Background(), 30*time.Second)
	if n, err := h.service.BackfillSnapshotSecretEncryption(backfillCtx); err != nil {
		app.Logger.Error("encrypting snapshot config secrets at rest failed", "encrypted", n, "error", err)
	} else if n > 0 {
		app.Logger.Info("snapshot config secrets encrypted at rest", "rows", n)
	}
	cancelBackfill()

	// Agent transfer endpoints. Public by necessity — an agent streams an
	// archive here without a McHarbor session — but not unauthenticated:
	// each handler calls agentArchiveTransfers.consume(transferId,
	// Authorization), a one-shot bearer token minted per transfer. A
	// permission gate would have no session user to resolve.
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/container-backups/internal/transfers/{transferId}", h.HandleRestoreTransfer)
		r.Put("/container-backups/internal/agent-archives/{transferId}", h.HandleAgentArchiveUpload, apispec.Exempt("agent transfer: a one-shot bearer token minted per transfer is the credential"))
		r.Get("/container-backups/internal/agent-archives/{transferId}", h.HandleAgentArchiveDownload)
	})

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/containers/{id}/backups", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/options", h.HandleOptions, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/run", h.HandleRunAdhoc, apispec.Scoped(), apispec.Request(RunBackupInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreRun)).Post("/restore-upload", h.HandleRestoreUpload, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreRun)).Post("/upload", h.HandleStageUpload, apispec.Scoped(), apispec.Response(BackupRun{}))
		})
		r.Route("/container-backups", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/", h.HandleListPlans, apispec.Scoped(), apispec.Query("containerId", "Only plans for this container."), apispec.Response([]BackupPlan{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/", h.HandleCreatePlan, apispec.Scoped(), apispec.Request(CreateBackupPlanInput{}), apispec.Response(BackupPlan{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/runs", h.HandleListRuns, apispec.Scoped(), apispec.Query("containerId", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/provider-runs", h.HandleListProviderRuns, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/provider-plans", h.HandleListProviderPlans, apispec.Scoped())
			// The single-run read the restore dialog polls for live
			// progress. Its absence made every GET on this path answer
			// 405 (chi sees the Delete/Post verbs below and rejects the
			// method), which left the dialog with no progress and no
			// completion.
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.runEnvironment))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/runs/{runId}", h.HandleRun)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsDownload)).Get("/runs/{runId}/download", h.HandleDownloadRun)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreView)).Post("/runs/{runId}/restore-options", h.HandleRestoreOptions, apispec.Request(RestoreBackupOptionsInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreView)).Post("/runs/{runId}/restore-filesystem-entries", h.HandleRestoreFilesystemEntries, apispec.Request(RestoreFilesystemEntriesInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreView)).Post("/runs/{runId}/restore-mount-entries", h.HandleRestoreMountEntries, apispec.Request(RestoreMountEntriesInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreView)).Post("/runs/{runId}/restore-config-entries", h.HandleRestoreConfigEntries, apispec.Request(RestoreConfigEntriesInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/runs/{runId}/cancel", h.HandleCancelRun)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsDelete)).Delete("/runs/{runId}", h.HandleDeleteRun)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermRestoreRun)).Post("/runs/{runId}/restore", h.HandleRestoreRun, apispec.Request(RestoreBackupInput{}), apispec.Response(BackupRun{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/runs/{runId}/destinations/{destinationId}/retry-upload", h.HandleRetryDestinationUpload)
			})
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.planEnvironment))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Put("/{planId}", h.HandleUpdatePlan, apispec.Request(UpdateBackupPlanInput{}), apispec.Response(BackupPlan{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsDelete)).Delete("/{planId}", h.HandleDeletePlan)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/{planId}/run", h.HandleRunPlan, apispec.Response(BackupRun{}))
			})
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/admin/relink-all", h.HandleRelinkAllContainerLinks)

			// Snapshot configuration. These rows decide whether a
			// backup freezes the filesystem or pauses the container,
			// so they are gated by the same keys as the plans that
			// consume them: reading is backups.view, changing is
			// backups.manage, removing is backups.delete.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/snapshot-configs", h.HandleListSnapshotConfigs, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/snapshot-configs", h.HandleCreateSnapshotConfig, apispec.Scoped())
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.snapshotConfigEnvironment))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Put("/snapshot-configs/{id}", h.HandleUpdateSnapshotConfig)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsDelete)).Delete("/snapshot-configs/{id}", h.HandleDeleteSnapshotConfig, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsManage)).Post("/snapshot-configs/{id}/probe", h.HandleProbeSnapshotConfig)
			})
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/snapshot-drivers", h.HandleDetectSnapshotDrivers, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermBackupsView)).Get("/snapshots/recent", h.HandleRecentSnapshots, apispec.Scoped(), apispec.Query("limit", ""))
		})
	})
}

// newMountedHandler builds the HTTP handler with what main injects. The
// handler's service is not the scheduler's, so it needs its own recreator;
// without one every config restore from the UI is refused as unsupported.
func newMountedHandler(app *router.AppDeps, runtime any, recreator ContainerRecreator) *Handler {
	h := NewHandler(app)
	if runtime != nil {
		h.SetPluginStorageAdapter(runtime)
	}
	if recreator != nil {
		h.SetContainerRecreator(recreator)
	}
	return h
}
