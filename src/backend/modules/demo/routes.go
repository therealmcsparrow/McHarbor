// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers /demo and returns the service so main can attach the
// runtime once it has started.
func Mount(app *router.AppDeps, restarter Restarter) *Service {
	svc := NewService(app.DB, app.Config.DataDir, app.Logger, restarter)
	h := NewHandler(app, svc)
	demoKey := rbac.RequireGlobalPermission(app.RBACService, rbac.PermSystemDemo)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/demo", func(r *apispec.Router) {
			r.Get("/status", h.HandleStatus,
				apispec.Summary("Whether the instance runs in demo mode"),
				apispec.Response(Status{}))
			r.With(demoKey).Post("/enter", h.HandleEnter,
				apispec.Summary("Build or reuse demo.db and restart into demo mode"),
				apispec.Request(ChangeRequest{}),
				apispec.Response(Outcome{}))
			r.With(demoKey).Post("/exit", h.HandleExit,
				apispec.Summary("Restart onto the real database; demo.db is kept"),
				apispec.Response(Outcome{}))
			r.With(demoKey).Post("/reset", h.HandleReset,
				apispec.Summary("Discard the demo estate; inside demo mode a fresh one is built for the next boot"),
				apispec.Request(ChangeRequest{}),
				apispec.Response(Outcome{}))
		})
	})
	return svc
}
