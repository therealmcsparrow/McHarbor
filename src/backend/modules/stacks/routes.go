// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers stack module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Tells the vault how to make a stack run with a rotated value. The
	// registry is on AppDeps because core/secrets cannot import this package.
	app.SecretAppliers.Register("stack", secretApplier{service: h.service})

	// GitOps deploys a promotion through DeployOwnedStack / RemoveOwnedStack and
	// looks the service up by name, because modules never import one another.
	app.RegisterService("stacks", h.service)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/stacks", func(r *apispec.Router) {
			r.With(rbac.RequireAnyStackPermission(app.RBACService, rbac.PermStacksView)).Get("/", h.HandleList, apispec.Scoped(), apispec.Response([]Stack{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/", h.HandleCreate, apispec.Request(CreateRequest{}), apispec.Response(Stack{}))
			r.With(rbac.RequireAnyStackPermission(app.RBACService, rbac.PermStacksView)).Post("/check-updates", h.HandleCheckImageUpdates, apispec.Scoped(), apispec.Request(CheckImageUpdatesRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/adopt/preview", h.HandleAdoptPreview, apispec.Scoped(), apispec.Request(AdoptPreviewRequest{}),
				apispec.Query("reveal", "Return credential-looking values in the clear; requires containers.manage and is audit-logged."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/adopt", h.HandleAdopt, apispec.Scoped(), apispec.Request(AdoptRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksView)).Get("/links", h.HandleGetContainerLink, apispec.Scoped(), apispec.Query("containerId", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/links", h.HandleLinkContainer, apispec.Scoped(), apispec.Request(LinkContainerRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Delete("/links/{containerId}", h.HandleUnlinkContainer, apispec.Scoped())

			r.Route("/{name}", func(r *apispec.Router) {
				// A stack lives in one environment but is addressed by name alone, so the
				// gates below decide on the stack's own environment, not the caller's ?env=.
				r.Use(rbac.PinEnvToResource(h.stackEnvironment))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/", h.HandleGetDetail, apispec.Scoped(), apispec.Response(Stack{}))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Put("/", h.HandleUpdate, apispec.Request(UpdateRequest{}), apispec.Response(Stack{}))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksDelete, "name")).Delete("/", h.HandleDelete, apispec.Scoped())
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/up", h.HandleUp, apispec.Scoped())
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/stop", h.HandleStop, apispec.Scoped())
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/down", h.HandleDown, apispec.Scoped())
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/restart", h.HandleRestart, apispec.Scoped())
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/update", h.HandleManagedUpdate)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/reinstall", h.HandleReinstall)

				// Phase 4 of the RBAC roadmap: assign owner to a stack.
				// Gated by users.manage (the broader admin right) since
				// ownership reassignment is a property of the user
				// model, not of the stack.
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Patch("/owner", h.HandleSetOwner, apispec.Request(SetOwnerRequest{}))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/compose", h.HandleGetCompose, apispec.Query("reveal", "Return masked environment values in the clear; requires stacks.manage on the stack and is audit-logged."))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/logs", h.HandleGetLogs, apispec.Scoped(), apispec.Query("tail", ""))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/containers", h.HandleGetContainers, apispec.Scoped())
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/env-vars", h.HandleGetEnvVars, apispec.Response(map[string]string{}), apispec.Query("reveal", "Return masked values in the clear; requires stacks.manage on the stack and is audit-logged."))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Put("/env-vars", h.HandleUpdateEnvVars, apispec.Request(map[string]string{}))
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/prune", h.HandlePrune, apispec.Scoped())

				// Webhooks
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/webhooks", h.HandleListWebhooks)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/webhooks", h.HandleCreateWebhook, apispec.Request(CreateStackWebhookInput{}))
				r.Route("/webhooks/{id}", func(r *apispec.Router) {
					r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Put("/", h.HandleUpdateWebhook, apispec.Request(UpdateStackWebhookInput{}))
					r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Delete("/", h.HandleDeleteWebhook)
					r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/test", h.HandleTestWebhook)
				})
			})
		})
	})
}
