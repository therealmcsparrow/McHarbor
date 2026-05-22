// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers metrics module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/metrics", func(r chi.Router) {
			r.Get("/host", h.HandleHostInfo)
			r.Get("/containers", h.HandleContainerStats)
			r.Get("/containers/{id}/stream", h.HandleContainerStatsStream)
		})
	})
}
