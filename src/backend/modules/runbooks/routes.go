// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// poolRuntime adapts the Docker client pool to the narrow runtime a step may
// reach, so nothing in this module holds a whole Docker client.
func poolRuntime(pool *docker.ClientPool) func(envID string) (Runtime, error) {
	return func(envID string) (Runtime, error) { return pool.Get(envID) }
}

// Mount registers the runbook routes and returns the service, so the incident
// module can read runs as a timeline source.
func Mount(app *router.AppDeps) *Service {
	svc := &Service{
		Store:  NewStore(app.DB),
		Authz:  app.RBACService,
		Audit:  app.AuditLog,
		Logger: app.Logger,
	}
	if app.DockerPool != nil {
		svc.RuntimeFor = poolRuntime(app.DockerPool)
	}
	if app.PluginRoutes != nil {
		svc.Plugins = RouterGateway{Routes: app.PluginRoutes, Capabilities: plugincaps.Default}
	}
	svc.Store.Requires = svc.requiredFor
	h := NewHandler(app, svc)

	view := rbac.RequirePermission(app.RBACService, rbac.PermRunbooksView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermRunbooksManage)
	run := rbac.RequirePermission(app.RBACService, rbac.PermRunbooksRun)
	automate := rbac.RequirePermission(app.RBACService, rbac.PermRunbooksAutomate)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/runbooks", func(r *apispec.Router) {
			r.With(view).Get("/", h.HandleList, apispec.Paginated(), apispec.Scoped())
			r.With(view).Get("/catalog", h.HandleCatalog, apispec.Response(CatalogView{}))
			r.With(automate).Get("/principals", h.HandlePrincipals, apispec.Response([]ServicePrincipal{}))
			// The one route the offline read cache serves, which is why it
			// returns whole runbooks with their steps in a single response.
			r.With(view).Get("/for", h.HandleResolve, apispec.Scoped(),
				apispec.Query("sourceType", ""), apispec.Query("conditionKey", ""),
				apispec.Query("alertId", ""), apispec.Query("alertEventId", ""),
				apispec.Query("container", ""), apispec.Query("stack", ""),
				apispec.Query("kind", "container, stack or an attach kind a loaded plugin declares; answered with id."),
				apispec.Query("id", "The attached object's id, in the form the declaring plugin lists it."),
				apispec.Response(ResolvedRunbooks{}))
			// The plugin's own list route answers, as the operator, so the
			// plugin's view key decides what a selector may offer.
			r.With(view).Get("/targets", h.HandleTargets, apispec.Scoped(),
				apispec.Query("kind", "A target kind a loaded plugin lists, as GET /runbooks/catalog reports under targets."),
				apispec.Query("cluster", "The cluster id, for a target kind whose list is scoped to one."),
				apispec.Response([]TargetOption{}))
			r.With(manage).Post("/", h.HandleSave, apispec.Request(RunbookInput{}), apispec.Response(Runbook{}))

			r.Route("/runs", func(r *apispec.Router) {
				r.With(view).Get("/", h.HandleListRuns, apispec.Paginated(), apispec.Scoped(),
					apispec.Query("incidentId", ""),
					apispec.Query("startedBy", "Only runs started by this user or service principal id."),
					apispec.Query("trigger", "manual, alert or schedule."))
				r.With(run).Post("/", h.HandleStartRun, apispec.Request(StartInput{}), apispec.Response(Run{}))
				r.Group(func(r *apispec.Router) {
					r.Use(rbac.PinEnvToResource(h.runEnvironment))
					r.With(view).Get("/{id}", h.HandleGetRun, apispec.Response(Run{}))
					// runbooks.run is permission to follow a runbook. The step's own
					// permission is checked inside the service, per step, against
					// this operator — this gate is not that check.
					r.With(run).Post("/{id}/steps/{stepId}", h.HandleRecordStep,
						apispec.Request(StepInputResult{}), apispec.Response(StepResult{}))
					r.With(run).Post("/{id}/finish", h.HandleFinishRun,
						apispec.Request(FinishInput{}), apispec.Response(Run{}))
				})
			})

			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.runbookEnvironment))
				r.With(view).Get("/{id}", h.HandleGet, apispec.Response(Runbook{}))
				// An export reads, so it takes view: the people who follow a runbook are
				// the ones who need to hand it to somebody.
				r.With(view).Get("/{id}/export", h.HandleExport, apispec.Response(RunbookExport{}))
				r.With(manage).Put("/{id}", h.HandleSave, apispec.Request(RunbookInput{}), apispec.Response(Runbook{}))
				r.With(manage).Delete("/{id}", h.HandleDelete)
				// runbooks.automate decides which service principal acts; every
				// step is still checked against that principal's own roles.
				r.With(automate).Put("/{id}/automation", h.HandleSetAutomation,
					apispec.Request(AutomationInput{}), apispec.Response(Runbook{}))
			})
		})
	})

	return svc
}
