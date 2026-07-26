// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers GitOps routes on the protected router.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Register the service so handlers in other modules can look it up.
	app.RegisterService("gitops", h.service)

	// Launch the scheduler as a background process.
	scheduler := NewScheduler(h.service, 60*time.Second)
	scheduler.Start()
	h.service.SetScheduler(scheduler)

	// Protected routes — operators manage pipelines, approve promotions.
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/gitops", func(r chi.Router) {
			r.Get("/pipelines", h.HandleListPipelines)
			r.Post("/pipelines", h.HandleCreatePipeline)
			r.Route("/pipelines/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGetPipeline)
				r.Put("/", h.HandleUpdatePipeline)
				r.Delete("/", h.HandleDeletePipeline)
				r.Post("/promote", h.HandlePromoteStage)
				r.Get("/promotions", h.HandleListPromotions)
			})

			r.Get("/approvals", h.HandleListApprovals)
			r.Route("/approvals/{id}", func(r chi.Router) {
				r.Post("/approve", h.HandleResolveApprovalFor("approved"))
				r.Post("/reject", h.HandleResolveApprovalFor("rejected"))
			})

			r.Get("/previews", h.HandleListPRPreviews)
		})
	})

	// Public webhook route is mounted under a different prefix to avoid
	// the chi "Mount on existing path" panic. The handler validates the
	// GitHub/GitLab/Gitea signature in HandlePullRequestWebhook.
	app.RegisterPublicRoutes(func(r chi.Router) {
		r.Route("/webhooks/gitops", func(r chi.Router) {
			r.Post("/pr", h.HandlePullRequestWebhook)
		})
	})
}
