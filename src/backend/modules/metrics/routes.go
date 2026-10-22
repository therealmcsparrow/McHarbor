// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers metrics module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/metrics", func(r *apispec.Router) {
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermMetricsScrape)).Get("/", h.HandleScrape,
				apispec.Summary("Scrape McHarbor's own metrics"), apispec.Text(),
				apispec.Describe("Prometheus text exposition of the control plane itself: HTTP, agent transport, background work, runtime and plugin state. Not wrapped in the JSON envelope. Every value is read from memory; a scrape performs no Docker, agent or database call."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/host", h.HandleHostInfo, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/host/fs", h.HandleHostFS,
				apispec.Scoped(), apispec.Response(HostFSStatus{}),
				apispec.Describe("Root filesystem of the host behind the environment with its freshness: ok, stale (last good sample kept), unknown or unsupported. Read from the tracker and the stored capacity series; it makes no Docker call, so it answers while the daemon is down."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/containers/{id}/stream", h.HandleContainerStatsStream, apispec.SSE(), apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/stats-path", h.HandleStatsPath, apispec.Response(StatsPath{}), apispec.Scoped())

			// Device inventory (GPUs, physical NICs, USB and block
			// devices) for the host running McHarbor. Gated by host.view
			// like the rest of this subtree — it answers "what is in this
			// machine", which is host information, not container
			// information.
			//
			// A standalone GET /gpu used to sit beside this. It was
			// superseded: enrichGPUs folds the same nvidia-smi telemetry
			// into this inventory, and this is the route the UI calls.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/hardware", h.HandleHardware)

			// Stored history. These read the metric tables only and never
			// touch Docker, so they stay available when the daemon is down.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/host/history", h.HandleHostHistory, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/containers/{id}/history", h.HandleContainerHistory, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostManage)).Post("/host/prune", h.HandleHostPrune, apispec.Scoped(), apispec.Request(PruneRequest{}))

			// Capacity forecasting. Both reads are host.view like the rest of
			// this subtree: a projection is a read of data that key already
			// grants, and environments.view is global-scoped, so reusing it for
			// a per-environment capacity read would widen the scope.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/capacity", h.HandleCapacity,
				apispec.Scoped(), apispec.Response(CapacityReport{}),
				apispec.Describe("Disk and storage-destination projections off the retained hourly capacity series. A subject with too little history, too noisy a series or no upward trend is returned with a named refusal instead of a date."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/capacity/idle", h.HandleCapacityIdle,
				apispec.Scoped(), apispec.Query("days", ""), apispec.Response(IdleReport{}),
				apispec.Describe("Reclaim candidates: containers stopped, or running with no CPU and no traffic, for longer than the threshold. Read-only — the reclaim action is the existing host prune."))
		})
	})
}
