// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

type handler struct {
	app    *router.AppDeps
	svc    *Service
	runner *Runner
}

// PackSummary is one framework as the packs list returns it: the mapping, plus
// the last run if the pack has ever been run. LatestRun is absent rather than
// zeroed when it has not, so the UI says "never run" instead of showing an empty
// control table that reads as nothing being wrong.
type PackSummary struct {
	Framework Framework `json:"framework"`
	LatestRun *Run      `json:"latestRun,omitempty"`
}

// StartedRun is the answer to a run request. The sweep continues after the
// response, so the operator is given the row to watch rather than a wait.
type StartedRun struct {
	RunID string `json:"runId"`
}

// handleList returns every pack with its latest run. Controls are included so the
// frontend can render a pack's whole mapping — the covered and the not covered —
// before any run exists.
func (h *handler) handleList(w http.ResponseWriter, r *http.Request) {
	out := make([]PackSummary, 0, len(Frameworks()))
	for _, f := range Frameworks() {
		s := PackSummary{Framework: f}
		run, ok, err := h.svc.LatestRun(r.Context(), f.Key)
		if err != nil {
			h.app.Logger.Error("compliance packs: reading the latest run failed", "framework", f.Key, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if ok {
			s.LatestRun = &run
		}
		out = append(out, s)
	}
	response.OK(w, out)
}

func (h *handler) handleListRuns(w http.ResponseWriter, r *http.Request) {
	framework := chi.URLParam(r, "framework")
	if _, ok := FrameworkByKey(framework); !ok {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	page, perPage := response.ParsePagination(r)
	runs, total, err := h.svc.ListRuns(r.Context(), framework, perPage, (page-1)*perPage)
	if err != nil {
		h.app.Logger.Error("compliance packs: listing runs failed", "framework", framework, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.Paginated(w, runs, int64(total), page, perPage)
}

func (h *handler) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	detail, ok, err := h.svc.RunDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.app.Logger.Error("compliance packs: reading a run failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if !ok {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, detail)
}

// handleStartRun records the run and sweeps in the background on the application's
// context, so a ten-minute sweep does not hold a request open and a shutdown
// cancels it rather than leaking a goroutine.
func (h *handler) handleStartRun(w http.ResponseWriter, r *http.Request) { //nolint:contextcheck // the sweep must outlive the request, so it runs on the application context that shutdown cancels
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	framework := chi.URLParam(r, "framework")
	envID := response.ParseEnvID(r)
	runID, err := h.runner.Begin(r.Context(), framework, envID, TriggerManual, user.ID)
	switch {
	case errors.Is(err, ErrUnknownFramework), errors.Is(err, ErrFrameworkNotRunnable):
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	case err != nil:
		h.app.Logger.Error("compliance packs: starting a run failed", "framework", framework, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	details, err := json.Marshal(map[string]string{"framework": framework, "environmentId": envID, "runId": runID})
	if err != nil {
		h.app.Logger.Error("compliance packs: marshalling audit details failed", "error", err)
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "compliance.pack.run",
		Severity:   "info",
		EntityType: "compliance_pack_run",
		EntityID:   runID,
		EntityName: framework,
		Details:    string(details),
	})
	ctx := h.app.ContextOrBackground()
	go func() {
		if err := h.runner.Execute(ctx, runID, framework, envID); err != nil {
			h.app.Logger.Warn("compliance packs: run failed", "run", runID, "error", err)
		}
	}()
	response.Created(w, StartedRun{RunID: runID})
}
