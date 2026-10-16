// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the instance backup routes. Downloading takes manage plus a
// step-up window, because an export is every secret in the instance at once;
// restoring takes its own key, and restoring audit history an approved elevation.
func Mount(app *router.AppDeps, svc *Service, uploader Uploader) {
	h := NewHandler(app, svc, uploader)
	view := rbac.RequirePermission(app.RBACService, rbac.PermInstanceBackupView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermInstanceBackupManage)
	restore := rbac.RequirePermission(app.RBACService, rbac.PermInstanceBackupRestore)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/instance-backups", func(r *apispec.Router) {
			r.With(view).Get("/", h.HandleList, apispec.Paginated(), apispec.Response([]Export{}))
			r.With(manage).Post("/", h.HandleCreate, apispec.Request(CreateExportInput{}), apispec.Response(Export{}))
			r.With(manage).Post("/key-file", h.HandleKeyFile, apispec.Response(KeyFileResult{}))
			r.With(view).Get("/schedules", h.HandleListSchedules, apispec.Response([]Schedule{}))
			r.With(manage).Post("/schedules", h.HandleCreateSchedule, apispec.Request(ScheduleInput{}), apispec.Response(Schedule{}))
			r.With(manage).Put("/schedules/{id}", h.HandleUpdateSchedule, apispec.Request(ScheduleInput{}), apispec.Response(Schedule{}))
			r.With(manage).Delete("/schedules/{id}", h.HandleDeleteSchedule)
			r.With(manage).Post("/schedules/{id}/run", h.HandleRunSchedule, apispec.Response(Schedule{}))
			r.With(restore).Put("/uploads", h.HandleUpload, apispec.Response(UploadResult{}))
			r.With(restore).Post("/restore/plan", h.HandlePlan, apispec.Request(RestoreInput{}), apispec.Response(core.Plan{}))
			r.With(restore).Post("/restore", h.HandleRestore, apispec.Request(RestoreInput{}), apispec.Response(RestoreStatus{}))
			r.With(view).Get("/restore/status", h.HandleRestoreStatus, apispec.Response(RestoreStatus{}))
			r.With(view).Get("/restore/ledger", h.HandleLedger, apispec.Response([]core.LedgerEntry{}))
			r.With(view).Get("/{id}", h.HandleGet, apispec.Response(Export{}))
			r.With(manage).Get("/{id}/download", h.HandleDownload, apispec.Binary())
			r.With(manage).Post("/{id}/verify", h.HandleVerify, apispec.Request(VerifyInput{}), apispec.Response(Export{}))
			r.With(manage).Delete("/{id}", h.HandleDelete)
		})
	})
}
