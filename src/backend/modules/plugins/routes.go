// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers plugin management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/plugins", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleInstall)
			r.Get("/{id}", h.HandleGet)
			r.Put("/{id}", h.HandleUpdateConfig)
			r.Delete("/{id}", h.HandleUninstall)
			r.Post("/{id}/toggle", h.HandleToggle)
		})
	})
}
