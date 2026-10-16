// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coresecrets "github.com/therealmcsparrow/mcharbor/core/secrets"
)

// Mount registers the secrets vault routes.
//
// Every route carries a permission gate. Two of them are worth reading twice:
//
//   - The `manage` routes gate on `secrets.manage`, but the scope arrives in
//     the request body, which middleware cannot see. The extra
//     `secrets.manage_global` requirement for a global-scoped secret is
//     enforced in the service (`requireGlobalAuthority`). Removing it there
//     removes it entirely.
//   - `/apply` gates on `secrets.apply`, not `secrets.manage`. Rotating a value
//     and restarting whatever runs it are different authorities, and one key
//     covering both would hand the second out silently with the first.
//   - `/reveal` gates on `secrets.reveal`, which ships in no role preset. It is
//     reachable only through an approved, time-bounded elevation, and — because
//     `core/rbac/rbac.go` does not fold elevations into the API-key path — only
//     from an interactive session. That is deliberate. Do not "fix" it.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/secrets", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsView)).Get("/", h.HandleList, apispec.Scoped(), apispec.Paginated(), apispec.Query("group", ""), apispec.Query("includeSystem", ""), apispec.Query("search", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsManage)).Post("/", h.HandleCreate, apispec.Scoped(), apispec.Request(coresecrets.CreateRequest{}), apispec.Response(coresecrets.Secret{}), apispec.Describe("With generate=true, data is instead {secret, value}: the generated value is returned once, here only."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsManage)).Post("/generate", h.HandleGenerate, apispec.Request(coresecrets.GenerateRequest{}))

			r.Route("/{id}", func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.secretEnvironment))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsView)).Get("/", h.HandleGet, apispec.Response(coresecrets.Secret{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsManage)).Put("/", h.HandleRotate, apispec.Scoped(), apispec.Request(coresecrets.RotateRequest{}), apispec.Response(coresecrets.Secret{}), apispec.Describe("With generate=true, data is instead {secret, value}: the generated value is returned once, here only."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsManage)).Patch("/", h.HandlePatch, apispec.Scoped(), apispec.Request(coresecrets.PatchRequest{}), apispec.Response(coresecrets.Secret{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsDelete)).Delete("/", h.HandleDelete, apispec.Query("force", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsView)).Get("/references", h.HandleReferences)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsView)).Get("/impact", h.HandleImpact)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsApply)).Post("/apply", h.HandleApply, apispec.Request(coresecrets.ApplyRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsManage)).Post("/references/prune", h.HandlePruneReferences, apispec.Request(coresecrets.ApplyRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsView)).Get("/versions", h.HandleVersions)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsManage)).Post("/rollback", h.HandleRollback, apispec.Scoped(), apispec.Request(coresecrets.RollbackRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSecretsReveal)).Post("/reveal", h.HandleReveal, apispec.Request(coresecrets.RevealRequest{}))
			})
		})
	})
}
