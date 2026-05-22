// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers agent module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Auth routes — no session middleware
	app.RegisterAuthRoutes(func(r chi.Router) {
		r.Get("/agent/ws", h.HandleAgentWS)
		r.Get("/agent/install/{token}", h.HandleInstallScript)
	})

	// Admin endpoints — session-protected
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", h.HandleListAgents)
			r.Route("/{envId}", func(r chi.Router) {
				r.Get("/status", h.HandleStatus)
				r.Post("/regenerate-token", h.HandleRegenerateToken)
				r.Post("/deploy", h.HandleDeploy)
				r.Post("/install-token", h.HandleCreateInstallToken)
			})
		})
	})
}
