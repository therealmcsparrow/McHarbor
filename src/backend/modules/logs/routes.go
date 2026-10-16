// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package logs

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers log streaming module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/logs", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLogsView)).Get("/{id}", h.HandleStreamLogs, apispec.SSE(), apispec.Scoped(),
				apispec.Query("tail", "Number of lines from the end to start with."),
				apispec.Query("stdout", "Include stdout; true by default."),
				apispec.Query("stderr", "Include stderr; true by default."),
				apispec.Query("follow", "Keep the stream open for new lines."),
				apispec.Query("cursor", "Resume after this event id; the Last-Event-ID header wins."))
		})
	})
}
