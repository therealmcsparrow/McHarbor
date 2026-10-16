// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfhost"
)

// Mount registers /host-services. The environment comes from ?env=, as every
// environment-scoped route does, so the environment-scoped keys are enforced
// against the host actually being touched. self reaches McHarbor's own machine.
func Mount(app *router.AppDeps, self selfhost.Runner) {
	h := NewHandler(app, self)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		MountRoutes(r, app, h)
	})
}

// MountRoutes declares the routes on r; split from Mount so tests serve the
// real table with the real RBAC middleware in front.
func MountRoutes(r *apispec.Router, app *router.AppDeps, h *Handler) {
	view := rbac.RequirePermission(app.RBACService, rbac.PermHostServicesView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermHostServicesManage)
	r.Route("/host-services", func(r *apispec.Router) {
		r.With(view).Get("/", h.HandleList,
			apispec.Scoped(),
			apispec.Summary("List the host's systemd services"),
			apispec.Describe("Native agent environments only. Loaded units merged with installed unit files. When the host cannot be managed, availability.reason says why (not_agent, not_native, offline, agent_outdated, no_systemd) and units is empty."),
			apispec.Response(UnitList{}))
		r.With(manage).Post("/daemon-reload", h.HandleDaemonReload,
			apispec.Scoped(),
			apispec.Summary("Run systemctl daemon-reload on the host"))
		r.With(view).Get("/{unit}/override", h.HandleGetOverride,
			apispec.Scoped(),
			apispec.Summary("Read a unit's effective definition and its drop-in override"),
			apispec.Describe("effective is `systemctl cat <unit>`; content is /etc/systemd/system/<unit>.d/override.conf, empty with exists=false until the first save."),
			apispec.Response(Override{}))
		r.With(manage).Put("/{unit}/override", h.HandlePutOverride,
			apispec.Scoped(),
			apispec.Summary("Write a unit's drop-in override"),
			apispec.Describe("Written atomically (temp file and rename, 0644, root-owned). reload runs daemon-reload after the write; verify runs systemd-analyze verify and returns its findings as verifyOutput without blocking the save."),
			apispec.Request(OverrideRequest{}),
			apispec.Response(OverrideResult{}))
		r.With(manage).Delete("/{unit}/override", h.HandleDeleteOverride,
			apispec.Scoped(),
			apispec.Summary("Remove a unit's drop-in override and reload systemd"),
			apispec.Describe("Deletes override.conf, then the .d directory when nothing else is left in it."))
		r.With(manage).Post("/{unit}/{action}", h.HandleAction,
			apispec.Scoped(),
			apispec.Summary("Start, stop, restart, enable or disable a unit"),
			apispec.Describe("action is start, stop, restart, enable or disable. stop and disable are refused with 409 err.host_services.protected on mcharbor-agent*, ssh/sshd, systemd-*, dbus*, docker and containerd. Restarting the agent's own unit answers warning=agent_restart: the connection drops and reconnects."),
			apispec.Response(ActionResult{}))
		mountSelfRoutes(r, app, h)
	})
}

// mountSelfRoutes declares /host-services/self: the machine McHarbor itself is
// installed on. It takes the same keys granted globally, as /system/self does
// for host updates, because a ?env= grant must not reach McHarbor's own host.
func mountSelfRoutes(r *apispec.Router, app *router.AppDeps, h *Handler) {
	view := rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfView)
	manage := rbac.RequireGlobalPermission(app.RBACService, rbac.PermHostSelfManage)
	r.Route("/self", func(r *apispec.Router) {
		r.With(view).Get("/", h.HandleSelfList,
			apispec.Summary("List the systemd services of the machine McHarbor runs on"),
			apispec.Describe("Reached through a privileged utility container (chroot /host) when McHarbor runs in a container, or directly when it runs natively as root. When that host cannot be managed, availability.reason is self_unreachable (availability.detail says why) or self_no_systemd, and units is empty."),
			apispec.Response(UnitList{}))
		r.With(manage).Post("/daemon-reload", h.HandleSelfDaemonReload,
			apispec.Summary("Run systemctl daemon-reload on the machine McHarbor runs on"))
		r.With(view).Get("/{unit}/override", h.HandleSelfGetOverride,
			apispec.Summary("Read a unit's effective definition and drop-in on the machine McHarbor runs on"),
			apispec.Response(Override{}))
		r.With(manage).Put("/{unit}/override", h.HandleSelfPutOverride,
			apispec.Summary("Write a unit's drop-in override on the machine McHarbor runs on"),
			apispec.Request(OverrideRequest{}),
			apispec.Response(OverrideResult{}))
		r.With(manage).Delete("/{unit}/override", h.HandleSelfDeleteOverride,
			apispec.Summary("Remove a unit's drop-in override on the machine McHarbor runs on and reload systemd"))
		r.With(manage).Post("/{unit}/{action}", h.HandleSelfAction,
			apispec.Summary("Start, stop, restart, enable or disable a unit on the machine McHarbor runs on"),
			apispec.Describe("stop and disable are refused with 409 err.host_services.protected on the environment list plus mcharbor* units. Restarting docker, containerd or a mcharbor unit is queued (--no-block) and answers warning=mcharbor_restart, since it takes McHarbor down with it."),
			apispec.Response(ActionResult{}))
	})
}
