// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker_info

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers docker info module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/docker", func(r chi.Router) {
			r.Get("/info", h.HandleSystemInfo)
		})
	})
}
