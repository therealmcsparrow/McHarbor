// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"encoding/json"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func jsonText(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// poolReader adapts the Docker client pool to the read-only evidence reader.
func poolReader(pool *docker.ClientPool) func(envID string) (DockerReader, error) {
	return func(envID string) (DockerReader, error) { return pool.Get(envID) }
}

// Mount registers the incident routes and returns the engine main starts. The
// model may be nil, in which case every narration refuses with no_model.
func Mount(app *router.AppDeps, model Model, coord Coordinator) *Engine {
	store := NewStore(app.DB)
	asm := &Assembler{DB: app.DB, Now: time.Now}
	if app.DockerPool != nil {
		asm.ReaderFor = poolReader(app.DockerPool)
	}
	svc := &Service{Store: store, Assembler: asm, Model: model, Audit: app.AuditLog, Logger: app.Logger}
	h := &Handler{app: app, svc: svc}
	view := rbac.RequirePermission(app.RBACService, rbac.PermIncidentsView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermIncidentsManage)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/incidents", func(r *apispec.Router) {
			r.With(view).Get("/", h.HandleList, apispec.Paginated(), apispec.Scoped(), apispec.Query("status", ""))
			r.With(view).Get("/settings", h.HandleGetSettings, apispec.Scoped(), apispec.Response(SettingsView{}))
			r.With(manage).Put("/settings", h.HandleSaveSettings, apispec.Request(SettingsInput{}), apispec.Response(SettingsView{}))
			r.With(view).Get("/findings", h.HandleFindings, apispec.Scoped(), apispec.Query("status", ""), apispec.Response([]Finding{}))
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.findingEnvironment))
				r.With(manage).Post("/findings/{fid}/explain", h.HandleExplainFinding, apispec.Response(Finding{}))
				r.With(manage).Post("/findings/{fid}/dismiss", h.HandleDismissFinding, apispec.Response(Finding{}))
			})
			r.With(view).Get("/sources", h.HandleSources, apispec.Response([]IncidentSource{}))
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.alertIncidentEnvironment))
				r.With(view).Get("/by-alert/{eventId}", h.HandleByAlert, apispec.Response(Incident{}))
			})
			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.incidentEnvironment))
				r.With(view).Get("/{id}", h.HandleGet, apispec.Response(Incident{}))
				r.With(manage).Post("/{id}/evidence", h.HandleRefreshEvidence, apispec.Response(Incident{}))
				r.With(manage).Post("/{id}/narrate", h.HandleNarrate, apispec.Response(Incident{}))
				r.With(manage).Post("/{id}/summarise", h.HandleSummarise, apispec.Response(Incident{}))
				r.With(manage).Post("/{id}/resolve", h.HandleResolve, apispec.Response(Incident{}))
			})
		})
	})

	return &Engine{
		Service:   svc,
		Detector:  &Detector{DB: app.DB, Store: store, Logger: app.Logger},
		Behaviour: &Behaviour{DB: app.DB},
		Coord:     coord,
		Logger:    app.Logger,
	}
}
