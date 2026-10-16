// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// RoleName is the singleton role the scheduler leases. It matches
// cluster.RoleCompliancePacks, kept as a string here so this module does not have
// to import the cluster package to name its own job.
const RoleName = "compliance-packs"

// Mount registers the pack routes and starts the daily sweep. Reads take
// compliance.view, the same key as the posture dashboard; starting a run takes
// compliance.manage, because a run reaches out to a live Docker daemon and
// reading a computed posture never does.
func Mount(app *router.AppDeps) {
	svc := NewService(app.DB, app.Logger)
	runner := NewRunner(svc, NewGatherer(app.DockerPool, app.DB, app.Logger), app.Logger)
	h := &handler{app: app, svc: svc, runner: runner}

	var coord leaseCoordinator
	if registered, ok := app.LookupService("coordinator"); ok {
		if c, ok := registered.(leaseCoordinator); ok {
			coord = c
		}
	}
	scheduler := NewScheduler(runner, svc, app.DB, coord, RoleName, app.Logger)
	go scheduler.Start(app.ContextOrBackground())
	app.RegisterService("compliance_packs", svc)

	view := rbac.RequirePermission(app.RBACService, rbac.PermComplianceView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermComplianceManage)
	attest := rbac.RequireGlobalPermission(app.RBACService, rbac.PermComplianceAttest)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		registerAttestationRoutes(r, h, view, attest)
		r.Route("/compliance/packs", func(r *apispec.Router) {
			r.With(view).Get("/", h.handleList,
				apispec.Response([]PackSummary{}))
			r.With(view).Get("/runs/{id}", h.handleRunDetail,
				apispec.Response(RunDetail{}))
			r.With(view).Get("/{framework}/runs", h.handleListRuns,
				apispec.Response([]Run{}))
			r.With(manage).Post("/{framework}/run", h.handleStartRun,
				apispec.Query("env", "Environment to check; empty means the default environment"),
				apispec.Response(StartedRun{}))
		})
	})
}
