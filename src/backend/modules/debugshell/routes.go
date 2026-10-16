// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// MountContainerRoutes registers /debug-shell inside the containers module's
// /{id} group; chi cannot mount /containers twice, so the module calls this.
func MountContainerRoutes(r *apispec.Router, app *router.AppDeps) {
	h := NewHandler(app)
	r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersDebugShell)).Post("/debug-shell", h.HandleInstall,
		apispec.Scoped(),
		apispec.Summary("Install a temporary debug shell into a shell-less container"),
		apispec.Describe("Uploads a static busybox to /.mcharbor-debug/busybox and returns the session the terminal WebSocket attaches with (?debugSession=). Refused with 409 when the container has a shell, is not running, or has a read-only root filesystem. Removed when the session ends, when it expires after 15 minutes, by the minute reaper, and at startup."),
		apispec.Response(Session{}))
	r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersView)).Get("/debug-shell", h.HandleStatus,
		apispec.Scoped(),
		apispec.Summary("List a container's open debug sessions"),
		apispec.Describe("leftBehind is true when a session is past expiry or its cleanup failed, so a busybox may still be in the container."),
		apispec.Response(SessionStatus{}))
	r.With(rbac.RequirePermission(app.RBACService, rbac.PermContainersDebugShell)).Delete("/debug-shell", h.HandleRemove,
		apispec.Scoped(),
		apispec.Summary("Remove the debug shell from a container now"),
		apispec.Query("session", "Close only this session; every open session on the container when empty."),
		apispec.Response(RemoveResult{}))
}
