// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers reconciler module routes with a responder of its own, reading
// Git sources through remotes.
func Mount(app *router.AppDeps, remotes RemoteAccessor) {
	MountWith(app, nil, remotes)
}

// MountWith registers reconciler routes over responder, the one the loop also
// holds, and returns the Git source syncer so the loop can run it each pass.
// remotes is the git module's service, wired by main.
func MountWith(app *router.AppDeps, responder *Responder, remotes RemoteAccessor) *SourceSyncer {
	h := NewHandler(app, responder)
	h.sources = NewSourceSyncer(app.DB, app.Encryption, remotes, app.AuditLog, app.Logger)

	// Rows written before config and spec were encrypted at rest are sealed once here.
	// Idempotent; a failure is logged and tolerated so it cannot stop the server.
	backfillCtx, cancelBackfill := context.WithTimeout(context.Background(), atRestBackfillTimeout)
	if n, err := h.service.BackfillEncryption(backfillCtx); err != nil {
		app.Logger.Error("reconciler: encrypting desired states at rest failed", "encrypted", n, "error", err)
	} else if n > 0 {
		app.Logger.Info("reconciler: desired states encrypted at rest", "rows", n)
	}
	cancelBackfill()
	view := rbac.RequirePermission(app.RBACService, rbac.PermReconcilerView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermReconcilerManage)
	enforce := rbac.RequirePermission(app.RBACService, rbac.PermReconcilerEnforce)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/reconciler", func(r *apispec.Router) {
			r.With(view).Get("/", h.HandleList, apispec.Paginated(), apispec.Scoped())
			r.With(manage).Post("/", h.HandleCreate, apispec.Request(CreateDesiredStateInput{}))
			r.With(view).Get("/report", h.HandleReport, apispec.Scoped(), apispec.Query("class", ""), apispec.Response([]ReportRow{}))

			r.With(view).Get("/settings", h.HandleGetSettings, apispec.Response(Settings{}))
			r.With(manage).Put("/settings", h.HandleSetConfirmPasses, apispec.Request(ConfirmPassesInput{}), apispec.Response(Settings{}))
			r.With(enforce).Put("/enforcement", h.HandleSetEnforcement, apispec.Request(EnforcementInput{}), apispec.Response(Settings{}))
			r.With(enforce).Post("/enforcement/reset", h.HandleResetBreaker, apispec.Response(Settings{}))

			r.With(view).Get("/proposals", h.HandleListProposals, apispec.Paginated(), apispec.Scoped(), apispec.Query("status", ""))
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.proposalEnvironment))
				r.With(view).Get("/proposals/{pid}", h.HandleGetProposal, apispec.Response(Proposal{}))
				r.With(manage).Post("/proposals/{pid}/approve", h.HandleApprove, apispec.Response(ProposalOutcome{}))
				r.With(manage).Post("/proposals/{pid}/reject", h.HandleReject, apispec.Request(RejectInput{}), apispec.Response(Proposal{}))
			})

			r.With(view).Get("/sources", h.HandleListSources, apispec.Response([]Source{}))
			r.With(manage).Post("/sources", h.HandleCreateSource, apispec.Request(SourceInput{}), apispec.Response(Source{}))
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.sourceEnvironment))
				r.With(manage).Put("/sources/{sid}", h.HandleUpdateSource, apispec.Request(SourceInput{}), apispec.Response(Source{}))
				r.With(manage).Delete("/sources/{sid}", h.HandleDeleteSource)
				r.With(manage).Post("/sources/{sid}/sync", h.HandleSyncSource, apispec.Response(SyncResult{}))
			})

			r.With(view).Get("/kinds", h.HandlePluginKinds, apispec.Response([]PluginKind{}))
			r.With(manage).Get("/kinds/{source}/{kind}/scopes", h.HandlePluginScopes, apispec.Response([]KindOption{}))
			r.With(manage).Get("/kinds/{source}/{kind}/subjects", h.HandlePluginSubjects, apispec.Query("env", ""), apispec.Response([]KindOption{}))
			r.With(manage).Get("/kinds/{source}/{kind}/options", h.HandlePluginOptions, apispec.Query("env", ""), apispec.Query("field", ""), apispec.Response([]KindOption{}))
			r.With(manage).Get("/kinds/{source}/{kind}/current", h.HandlePluginCurrent, apispec.Query("env", ""), apispec.Query("subject", ""))

			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.watchEnvironment))
				r.With(view).Get("/{id}", h.HandleGet, apispec.Response(DesiredState{}))
				r.With(manage).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateDesiredStateInput{}))
				r.With(manage).Delete("/{id}", h.HandleDelete)
				r.With(manage).Post("/{id}/reconcile", h.HandleReconcile)
				r.With(view).Get("/{id}/drift", h.HandleDrift)
				r.With(manage).Post("/{id}/propose", h.HandlePropose, apispec.Response(Proposal{}))
				r.With(enforce).Post("/{id}/enforce", h.HandleEnforce, apispec.Response(DesiredState{}))
				r.With(manage).Post("/{id}/pull-request", h.HandlePullRequest, apispec.Response(PullRequestResult{}))
			})
		})
	})
	return h.sources
}
