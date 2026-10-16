// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers GitOps routes on the protected router.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Register the service so handlers in other modules can look it up.
	app.RegisterService("gitops", h.service)

	// Wired after construction rather than passed to NewService: the preview
	// path must honour environments.manage against a real principal, attribute a
	// teardown it performs on its own, and reach the environments service
	// without importing it. The environments lookup is deferred to call time so
	// module mount order cannot decide whether previews work.
	h.service.SetRBAC(app.RBACService)
	h.service.SetAudit(app.AuditLog)
	h.service.SetEnvironmentResolver(func() (EnvironmentProvisioner, bool) {
		svc, ok := app.LookupService("environments")
		if !ok {
			return nil, false
		}
		prov, ok := svc.(EnvironmentProvisioner)
		return prov, ok
	})

	// A promotion deploys through the stacks module's owned-stack seam, fetched
	// into DATA_DIR and run on a background runner parented on the application
	// context, so shutdown cancels a deploy rather than orphaning it.
	h.service.SetStackResolver(func() (StackDeployer, bool) {
		svc, ok := app.LookupService("stacks")
		if !ok {
			return nil, false
		}
		deployer, ok := svc.(StackDeployer)
		return deployer, ok
	})
	if app.Config != nil {
		h.service.SetDataDir(app.Config.DataDir)
	}
	h.service.SetBackgroundContext(app.ContextOrBackground())
	h.service.PurgeStaleCheckouts()

	// Launch the scheduler as a background process. In HA mode, the
	// registered coordinator elects one node to poll each tick.
	scheduler := NewScheduler(h.service, 60*time.Second)
	if svc, ok := app.LookupService("coordinator"); ok {
		if coord, ok := svc.(schedulerCoordinator); ok {
			scheduler = NewSchedulerWithCoordinator(h.service, 60*time.Second, coord, coord.NodeID())
		}
	}
	// Demo mode never starts the poller: its pipelines point at forges that do
	// not exist, and its seeded previews must not be reaped under the viewer.
	if !coredemo.Active() {
		scheduler.Start()
	}
	h.service.SetScheduler(scheduler)

	// Protected routes — operators manage pipelines, approve promotions.
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/gitops", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsView)).Get("/pipelines", h.HandleListPipelines)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsManage)).Post("/pipelines", h.HandleCreatePipeline, apispec.Request(CreatePipelineInput{}))
			r.Route("/pipelines/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsView)).Get("/", h.HandleGetPipeline)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsManage)).Put("/", h.HandleUpdatePipeline, apispec.Request(UpdatePipelineInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsDelete)).Delete("/", h.HandleDeletePipeline)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsManage)).Post("/promote", h.HandlePromoteStage, apispec.Request(PromoteInput{}), apispec.Response(Promotion{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsView)).Get("/promotions", h.HandleListPromotions, apispec.Paginated())
			})

			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsView)).Get("/approvals", h.HandleListApprovals, apispec.Response([]Approval{}), apispec.Query("status", ""))
			r.Route("/approvals/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsApprove)).Post("/approve", h.HandleApprove, apispec.Request(ApprovalInput{}), apispec.Response(Approval{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsApprove)).Post("/reject", h.HandleReject, apispec.Request(ApprovalInput{}), apispec.Response(Approval{}))
			})

			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsView)).Get("/previews", h.HandleListPRPreviews, apispec.Query("pipelineId", ""))
			// Destroying a preview deletes an environment, so it is a delete
			// rather than a manage.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsDelete)).Delete("/previews/{id}", h.HandleDestroyPreview, apispec.Response(PRPreview{}))

			// The webhook secret is what authenticates every inbound delivery, so
			// reading whether one exists is a view and minting one is a manage.
			r.Route("/repos/{repoId}/webhook-secret", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsView)).Get("/", h.HandleWebhookSecretStatus, apispec.Response(WebhookSecretStatus{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsManage)).Post("/", h.HandleGenerateWebhookSecret, apispec.Response(WebhookSecretGenerated{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitopsManage)).Delete("/", h.HandleClearWebhookSecret)
			})
		})
	})

	// Public webhook route is mounted under a different prefix to avoid the chi
	// "Mount on existing path" panic. It carries no session because a forge
	// cannot present one.
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Route("/webhooks/gitops", func(r *apispec.Router) {
			r.Post("/pr", h.HandlePullRequestWebhook,
				apispec.Exempt("inbound webhook: authenticated by an HMAC-SHA256 over the raw request body, verified against the named repository's stored webhook secret before the payload is decoded or acted on; a repository with no secret is refused"),
				apispec.Query("repo", "The McHarbor repository id the delivery belongs to; required unless the body carries repoId."),
				apispec.Request(PullRequestInput{}))
		})
	})
}
