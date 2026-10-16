// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

// Mount registers auto-update policy module routes; bins is the agent module's
// served-binary lookup, wired by main.
func Mount(app *router.AppDeps, bins AgentBinaries) {
	h := NewHandler(app)
	dispatcher := notify.NewDispatcher(app.DB, app.Encryption)
	checker := NewSelfUpdateChecker(app.DB, dispatcher, app.Logger)
	if registered, ok := app.LookupService("coordinator"); ok {
		if coord, ok := registered.(checkerCoordinator); ok {
			checker = NewSelfUpdateCheckerWithCoordinator(app.DB, dispatcher, app.Logger, coord, coord.NodeID())
		}
	}
	if err := checker.Load(app.ContextOrBackground()); err != nil {
		app.Logger.Warn("self-update: load cached state failed", "error", err)
	}
	// Automatic install reuses the manual self-update path; automatic
	// agent updates talk to the live agent pool.
	imageForVersion = coredocker.ImageForVersion
	checker.SetApplier(func(ctx context.Context, targetImage string) error {
		_, err := coredocker.ApplyLocalSelfUpdate(ctx, targetImage)
		return err
	}, coredocker.CurrentMcHarborImage)
	releases := agentreleases.Shared(app)
	releasesHandler := agentreleases.NewHandler(app, releases)
	checker.SetAgentUpdater(NewAgentUpdater(app.AgentPool, app.Logger, releases, bins))
	checker.SetBusyChecker(DBBusyChecker(app.DB))
	h.SetSelfUpdateChecker(checker)
	if settings, err := checker.Settings(app.ContextOrBackground()); err == nil && settings.Enabled {
		checker.Start(app.ContextOrBackground())
	}
	if releases != nil {
		releases.RunScheduler(app.ContextOrBackground(), releaseNotifier(dispatcher, app), releaseLeader(app))
	}

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/updates", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/", h.HandleList, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Post("/", h.HandleCreate, apispec.Request(CreatePolicyInput{}), apispec.Response(Policy{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/check", h.HandleCheckUpdate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/state", h.HandleState)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/settings", h.HandleGetSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Put("/settings", h.HandleSaveSettings, apispec.Request(SelfUpdateSettings{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Post("/dismiss", h.HandleDismiss, apispec.Request(DismissRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/agents", h.HandleAgentInventory)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/agents/update", h.HandleUpdateAgents, apispec.Request(UpdateAgentsRequest{}))
			// The agent release store. Reading is updates.view; pulling, deleting,
			// choosing the target and the schedule are updates.manage, like the
			// McHarbor self-update settings they mirror. Rolling a target out to
			// agents stays behind environments.manage on /agents/update above.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/agents/releases", releasesHandler.HandleList, apispec.Response(agentreleases.Overview{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/agents/releases/settings", releasesHandler.HandleGetSettings, apispec.Response(agentreleases.Settings{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Put("/agents/releases/settings", releasesHandler.HandleSaveSettings, apispec.Request(agentreleases.Settings{}), apispec.Response(agentreleases.Settings{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Post("/agents/releases/check", releasesHandler.HandleCheck, apispec.Response(agentreleases.State{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Put("/agents/releases/target", releasesHandler.HandleSetTarget, apispec.Request(agentreleases.SetTargetRequest{}), apispec.Response(agentreleases.ActionResponse{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Post("/agents/releases/{version}/pull", releasesHandler.HandlePull, apispec.Query("build", "Pull this build (yyyyMMddHHmm) instead of the newest; an older build is pinned until another is pulled."), apispec.Response(agentreleases.ActionResponse{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Delete("/agents/releases/{version}", releasesHandler.HandleDelete, apispec.Query("build", "The stored build to delete; required once the version has more than one."), apispec.Response(agentreleases.ActionResponse{}))
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.policyEnvironment))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/{id}", h.HandleGet, apispec.Response(Policy{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Put("/{id}", h.HandleUpdate, apispec.Request(UpdatePolicyInput{}), apispec.Response(Policy{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesManage)).Delete("/{id}", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)).Get("/{id}/history", h.HandleHistory, apispec.Paginated())
			})
		})
	})
}
