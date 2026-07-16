// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers auto-update policy module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	dispatcher := notify.NewDispatcher(app.DB, app.Encryption)
	checker := NewSelfUpdateChecker(app.DB, dispatcher, app.Logger)
	if err := checker.Load(app.ContextOrBackground()); err != nil {
		app.Logger.Warn("self-update: load cached state failed", "error", err)
	}
	h.SetSelfUpdateChecker(checker)
	if settings, err := checker.Settings(app.ContextOrBackground()); err == nil && settings.Enabled {
		checker.Start(app.ContextOrBackground())
	}

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/updates", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/check", h.HandleCheckUpdate)
			r.Get("/state", h.HandleState)
			r.Get("/settings", h.HandleGetSettings)
			r.Put("/settings", h.HandleSaveSettings)
			r.Post("/dismiss", h.HandleDismiss)
			r.Get("/{id}", h.HandleGet)
			r.Put("/{id}", h.HandleUpdate)
			r.Delete("/{id}", h.HandleDelete)
			r.Get("/{id}/history", h.HandleHistory)
		})
	})
}
