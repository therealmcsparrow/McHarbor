// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers container module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/containers", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/stats/summary", h.HandleBulkStats)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Post("/check-updates", h.HandleCheckImageUpdates)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/prune", h.HandlePrune)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/", h.HandleCreate)

			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/", h.HandleInspect)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersDelete)).Delete("/", h.HandleRemove)

				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersDelete)).Post("/remove", h.HandleRemoveExtended)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/start", h.HandleStart)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/stop", h.HandleStop)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/restart", h.HandleRestart)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/pause", h.HandlePause)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/unpause", h.HandleUnpause)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/kill", h.HandleKill)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/update", h.HandleUpdate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/recreate", h.HandleRecreate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/network/connect", h.HandleNetworkConnect)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/network/disconnect", h.HandleNetworkDisconnect)

				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/logs", h.HandleLogs)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/stats", h.HandleStats)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/top", h.HandleTop)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/files", h.HandleListFiles)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/files/content", h.HandleGetFile)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Put("/files/content", h.HandleSaveFile)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Delete("/files/content", h.HandleDeleteFile)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/upload", h.HandleUploadFile)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/directory", h.HandleCreateDir)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/rename", h.HandleRenameFile)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersManage)).Post("/files/chmod", h.HandleChmod)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/services", h.HandleDetectServices)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Post("/shells", h.HandleDetectShells)
			})
		})
	})
}
