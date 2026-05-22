// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package blueprints

import (
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers blueprint module routes and seeds built-in blueprints from YAML files.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Seed built-in blueprints from YAML files on disk.
	blueprintsDir := filepath.Join(filepath.Dir(app.Config.DataDir), "blueprints")
	if err := h.service.Seed(blueprintsDir, app.Logger); err != nil {
		app.Logger.Error("blueprints: seed failed", "error", err)
	}

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/blueprints", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/{id}", h.HandleGet)
			r.Put("/{id}", h.HandleUpdate)
			r.Delete("/{id}", h.HandleDelete)
			r.Post("/{id}/deploy", h.HandleDeploy)
		})
	})
}
