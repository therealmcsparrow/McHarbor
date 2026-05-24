// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers auth module routes.
// Login, logout, and setup are public (no auth middleware).
// Session is protected (requires auth middleware).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Public auth routes (rate-limited, no auth middleware)
	app.RegisterAuthRoutes(func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", h.HandleStatus)
			r.Post("/login", h.HandleLogin)
			r.Post("/logout", h.HandleLogout)
			r.Post("/setup", h.HandleSetup)
		})
	})

	// Protected auth routes
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Get("/auth/session", h.HandleSession)
		r.Put("/auth/profile", h.HandleUpdateProfile)
		r.Put("/auth/preferences", h.HandleUpdatePreferences)
	})
}
