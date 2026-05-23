// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers workflows module routes using the default runtime trigger service.
func Mount(app *router.AppDeps) {
	MountWithTriggerService(app, NewTriggerService(app, NewHub()))
}

// MountWithTriggerService registers workflows routes with an explicit trigger service.
func MountWithTriggerService(app *router.AppDeps, ts *TriggerService) {
	hub := NewHub()
	h := NewHandler(app, hub)
	ts.hub = hub
	ts.handler = h
	ts.service = h.service

	// Wire link-out → link-in auto-trigger for both handler and trigger service instances
	linkOutCb := func(ctx context.Context, sourceWorkflowID, sourceNodeID string, msg Msg) {
		go ts.triggerLinkInWorkflows(ctx, sourceWorkflowID, sourceNodeID, msg)
	}
	h.service.SetLinkOutCallback(linkOutCb)
	ts.service.SetLinkOutCallback(linkOutCb)

	app.RegisterAuthRoutes(func(r chi.Router) {
		r.HandleFunc("/workflows/webhooks/*", h.HandleIncomingWebhook)
	})

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/workflow-nodes", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreNodesView)).Get("/", h.HandleListNodeAvailability)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreNodesManage)).Put("/{key}", h.HandleUpdateNodeAvailability)
		})

		r.Route("/workflows", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/runs", h.HandleListRuns)
			r.Get("/link-outputs", h.HandleListLinkOutputs)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGet)
				r.Put("/", h.HandleUpdate)
				r.Delete("/", h.HandleDelete)
				r.Post("/execute", h.HandleExecute)
				r.Get("/live", h.HandleLiveEvents)
			})
		})
	})

}
