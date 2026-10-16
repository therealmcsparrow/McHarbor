// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers system module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	h.hostUpdates.StartScheduler(context.Background())
	h.reboots.StartScheduler(context.Background())

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/system", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermTerminalAccess)).Get("/host-terminal/ws", h.HandleOSTerminalWS, apispec.WebSocket(), apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLogsView)).Get("/os-logs", h.HandleOSLogs, apispec.Scoped(), apispec.Query("source", ""), apispec.Query("tail", ""), apispec.Query("target", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesView)).Get("/os-info", h.HandleHostOSInfo, apispec.Scoped())
			mountHostDeviceRoutes(r, app, &hostDevicesHandler{app: app, lister: h.svc})
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesView)).Get("/os-updates/state", h.HandleHostUpdateState, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesView)).Get("/os-updates/check", h.HandleHostUpdateCheck, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesView)).Get("/os-updates/stream", h.HandleHostUpdateStream, apispec.SSE(), apispec.Query("job", ""), apispec.Query("offset", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesManage)).Put("/os-updates/schedule", h.HandleHostUpdateSchedule, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesManage)).Post("/os-updates/apply", h.HandleHostUpdateApply, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesView)).Get("/os-reboot", h.HandleHostRebootState, apispec.Scoped(), apispec.Response(HostRebootState{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesView)).Get("/os-reboot/inventory", h.HandleHostRebootInventory, apispec.Scoped(), apispec.Response(HostRebootInventory{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesReboot)).Post("/os-reboot", h.HandleHostRebootSchedule, apispec.Scoped(), apispec.Request(HostRebootRequest{}), apispec.Response(HostReboot{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostUpdatesReboot)).Delete("/os-reboot", h.HandleHostRebootCancel, apispec.Scoped())
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)).Get("/self/os-reboot", h.HandleSelfHostRebootState, apispec.Response(HostRebootState{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)).Get("/self/os-reboot/inventory", h.HandleSelfHostRebootInventory, apispec.Response(HostRebootInventory{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfReboot)).Post("/self/os-reboot", h.HandleSelfHostRebootSchedule, apispec.Request(HostRebootRequest{}), apispec.Response(HostReboot{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfReboot)).Delete("/self/os-reboot", h.HandleSelfHostRebootCancel)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)).Get("/self/os-info", h.HandleSelfHostOSInfo)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)).Get("/self/os-updates/state", h.HandleSelfHostUpdateState)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)).Get("/self/os-updates/check", h.HandleSelfHostUpdateCheck)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)).Get("/self/os-updates/stream", h.HandleSelfHostUpdateStream, apispec.SSE(), apispec.Query("job", ""), apispec.Query("offset", ""))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfManage)).Put("/self/os-updates/schedule", h.HandleSelfHostUpdateSchedule, apispec.Request(HostUpdateSchedule{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfManage)).Post("/self/os-updates/apply", h.HandleSelfHostUpdateApply, apispec.Request(OSUpdateApplyRequest{}), apispec.Response(HostUpdateJobStatus{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSystemManage)).Post("/restart", h.HandleRestart)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSystemManage)).Post("/reset", h.HandleReset, apispec.Request(ResetRequest{}))
		})
	})
}
