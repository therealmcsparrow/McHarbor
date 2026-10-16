// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
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

	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.HandleFunc("/workflows/webhooks/*", h.HandleIncomingWebhook)
	})

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/workflow-nodes", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreNodesView)).Get("/", h.HandleListNodeAvailability)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreNodesManage)).Put("/{key}", h.HandleUpdateNodeAvailability, apispec.Request(UpdateNodeAvailabilityInput{}))
		})

		r.Route("/workflows", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsView)).Get("/", h.HandleList, apispec.Paginated(), apispec.Query("status", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsManage)).Post("/", h.HandleCreate, apispec.Request(CreateInput{}), apispec.Response(Workflow{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsManage)).Post("/import", h.HandleImport, apispec.Request(WorkflowExport{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsView)).Get("/runs", h.HandleListRuns, apispec.Paginated(), apispec.Query("workflow_id", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsView)).Get("/link-outputs", h.HandleListLinkOutputs)
			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsView)).Get("/", h.HandleGet, apispec.Response(Workflow{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsManage)).Put("/", h.HandleUpdate, apispec.Request(UpdateInput{}), apispec.Response(Workflow{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsDelete)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsView)).Get("/export", h.HandleExport)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsRun)).Post("/execute", h.HandleExecute, apispec.SSE(), apispec.Request(ExecuteInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermWorkflowsView)).Get("/live", h.HandleLiveEvents, apispec.SSE())
			})
		})
	})

}
