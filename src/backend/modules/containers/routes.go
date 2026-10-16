// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/modules/debugshell"
)

// Mount registers container module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Tells the vault how to make a container run with a rotated value. The
	// registry is on AppDeps because core/secrets cannot import this package.
	app.SecretAppliers.Register("container", secretApplier{service: h.svc})

	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/containers/internal/move-transfers/{transferId}", h.HandleMoveTransfer)
	})

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/containers", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/", h.HandleList, apispec.Scoped(), apispec.Query("all", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/stats/summary", h.HandleBulkStats, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/health/summary", h.HandleHealthSummary, apispec.Scoped(), apispec.Query("days", ""), apispec.Query("from", ""), apispec.Query("limit", ""), apispec.Query("to", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Post("/check-updates", h.HandleCheckImageUpdates, apispec.Scoped(), apispec.Request(CheckImageUpdatesRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/prune", h.HandlePrune, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/", h.HandleCreate, apispec.Scoped(), apispec.Request(CreateRequest{}))

			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/", h.HandleInspect, apispec.Scoped(),
					apispec.Query("reveal", "Return credential-looking env values in the clear; requires containers.manage and is audit-logged."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersDelete)).Delete("/", h.HandleRemove, apispec.Scoped(),
					apispec.Query("force", "Kill a running container before removing it."),
					apispec.Query("v", "Also remove the container's anonymous volumes."))

				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersDelete)).Post("/remove", h.HandleRemoveExtended, apispec.Scoped(), apispec.Request(RemoveExtendedRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Post("/move/plan", h.HandleMovePlan, apispec.Scoped(), apispec.Request(MoveContainerPlanRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/move", h.HandleMove, apispec.Scoped(), apispec.Request(MoveContainerRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/move/stream", h.HandleMoveStream, apispec.SSE(), apispec.Scoped(), apispec.Request(MoveContainerRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/start", h.HandleStart, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/stop", h.HandleStop, apispec.Scoped(), apispec.Query("t", "Seconds to wait before killing the container."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/restart", h.HandleRestart, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/pause", h.HandlePause, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/unpause", h.HandleUnpause, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/kill", h.HandleKill, apispec.Scoped(), apispec.Query("signal", "Signal to send; SIGKILL when empty."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/update", h.HandleUpdate, apispec.Scoped(), apispec.Request(UpdateRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/rename", h.HandleRename, apispec.Scoped(), apispec.Request(RenameRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/recreate", h.HandleRecreate, apispec.Scoped(), apispec.Request(RecreateEditRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/network/connect", h.HandleNetworkConnect, apispec.Scoped(), apispec.Request(NetworkConnectRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/network/disconnect", h.HandleNetworkDisconnect, apispec.Scoped(), apispec.Request(NetworkDisconnectRequest{}))

				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/logs", h.HandleLogs, apispec.Scoped(),
					apispec.Query("tail", "Number of lines from the end."),
					apispec.Query("stdout", "Include stdout; true by default."),
					apispec.Query("stderr", "Include stderr; true by default."),
					apispec.Query("since", "Only lines after this RFC3339 time or Unix timestamp."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/stats", h.HandleStats, apispec.Scoped(), apispec.Query("stream", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/top", h.HandleTop, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/health/history", h.HandleHealthHistory, apispec.Scoped(), apispec.Query("limit", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/files", h.HandleListFiles, apispec.Scoped(), apispec.Query("path", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/files/content", h.HandleGetFile, apispec.Scoped(), apispec.Query("download", ""), apispec.Query("path", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Put("/files/content", h.HandleSaveFile, apispec.Scoped(), apispec.Query("path", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Delete("/files/content", h.HandleDeleteFile, apispec.Scoped(), apispec.Query("path", ""), apispec.Query("recursive", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/upload", h.HandleUploadFile, apispec.Scoped(), apispec.Query("path", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/directory", h.HandleCreateDir, apispec.Scoped(), apispec.Query("path", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/rename", h.HandleRenameFile, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/chmod", h.HandleChmod, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/services", h.HandleDetectServices, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Post("/shells", h.HandleDetectShells, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/diff", h.HandleDiff, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/exec", h.HandleExec, apispec.Scoped(), apispec.Request(ExecRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/exec/history", h.HandleExecHistory, apispec.Scoped(), apispec.Query("limit", ""))
				debugshell.MountContainerRoutes(r, app)
			})
		})
	})
}
