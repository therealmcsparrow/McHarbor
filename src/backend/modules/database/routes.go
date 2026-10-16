// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the database module's routes.
//
// The restarter is what puts a new database into service — McHarbor reads
// its connection once, at boot, so the switch is a container restart rather
// than a hot swap. Passing nil is supported and means "this install cannot
// restart itself"; the UI then tells the operator to do it by hand.
func Mount(app *router.AppDeps, restarter Restarter) {
	h := NewHandler(app, restarter)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/database", func(r *apispec.Router) {
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseView)).
				Get("/config", h.HandleGetConfig)
			// database.manage rather than view: the test dials an
			// arbitrary host from inside the container and reports the
			// driver's own diagnostics back.
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/test", h.HandleTest, apispec.Request(ConnectionInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/migration", h.HandleStartMigration, apispec.Request(MigrationRequest{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseView)).
				Get("/migration", h.HandleMigrationStatus)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/migration/cancel", h.HandleCancelMigration)
			// Reading the cleanup status is database.manage rather than
			// database.view: it names a path on the host filesystem, and the
			// only reason to look at it is to decide whether to delete it.
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Get("/cleanup", h.HandleCleanupStatus)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/cleanup", h.HandleCleanup)
			// database.manage rather than view for start and cancel: a full
			// sweep across every table is real load on the database every
			// other request shares.
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/check", h.HandleStartCheck)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseView)).
				Get("/check", h.HandleCheckStatus)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/check/cancel", h.HandleCancelCheck)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermDatabaseManage)).
				Post("/check/repair", h.HandleRepair)
		})
	})

	// First-run routes. Rate-limited, no auth middleware — the same group
	// POST /auth/setup lives in, for the same reason: they run before the
	// first account exists, so there is no session to authenticate and no
	// role to hold a permission. Each one refuses the moment a user exists,
	// which is the guard that replaces the permission.
	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Route("/database/setup", func(r *apispec.Router) {
			r.Get("/status", h.HandleSetupStatus)
			// pre-auth: no session and no role exist yet on first run; both
			// refuse the moment a user account exists.
			r.Post("/test", h.HandleSetupTest, apispec.Exempt("first-run: refuses once any account exists"), apispec.Request(ConnectionInput{}))
			r.Post("/apply", h.HandleSetupApply, apispec.Exempt("first-run: refuses once any account exists"), apispec.Request(ConnectionInput{}))
		})
	})
}
