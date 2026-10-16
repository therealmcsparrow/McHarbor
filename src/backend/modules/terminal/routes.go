// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package terminal

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers terminal module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/terminal", func(r *apispec.Router) {
			r.Get("/ws", h.HandleWS, apispec.WebSocket(), apispec.Scoped(), apispec.Query("container", ""), apispec.Query("shell", ""), apispec.Query("user", ""), apispec.Query("workDir", ""), apispec.Query("debugSession", "Attach to a debug shell installed by POST /containers/{id}/debug-shell; needs containers.debug_shell."))
		})
	})
}
