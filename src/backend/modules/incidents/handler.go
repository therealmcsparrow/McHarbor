// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the incident routes.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// SettingsInput is the body that changes an environment's narration consent.
type SettingsInput struct {
	EnvID        string `json:"envId"`
	NarrateOptIn bool   `json:"narrateOptIn"`
	Proactive    bool   `json:"proactive"`
	HourlyCap    int    `json:"hourlyCap"`
}

// SettingsView is an environment's settings with the model its evidence
// would go to, which the opt-in dialog states in plain words.
type SettingsView struct {
	Settings
	ModelConfigured bool       `json:"modelConfigured"`
	Model           *ModelInfo `json:"model,omitempty"`
}

func caller(r *http.Request) string {
	if u := auth.RequireAuth(r); u != nil {
		return u.Username
	}
	return ""
}

func (h *Handler) refuse(w http.ResponseWriter, r *http.Request, err error, what string) {
	reason := ""
	status := http.StatusConflict
	switch {
	case errors.Is(err, ErrNotOptedIn):
		reason = "not_opted_in"
	case errors.Is(err, ErrNoModel):
		reason = "no_model"
	case errors.Is(err, ErrInProgress):
		reason = "in_progress"
	case errors.Is(err, ErrCapReached):
		reason = "cap_reached"
	case errors.Is(err, ErrNotResolved):
		reason = "not_resolved"
	case errors.Is(err, ErrModelFailed):
		reason, status = "model_failed", http.StatusBadGateway
	default:
		h.app.Logger.Error("incidents: "+what+" failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if status == http.StatusBadGateway {
		h.app.Logger.Warn("incidents: "+what+": model failed", "error", err)
	}
	response.ErrCodeWithData(w, r, status, i18n.ErrInvalidBody, map[string]string{"reason": reason})
}

// HandleList lists incidents newest first.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != StatusOpen && status != StatusResolved {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	page, perPage := response.ParsePagination(r)
	items, total, err := h.svc.Store.List(r.Context(), response.ParseEnvID(r), status, page, perPage)
	if err != nil {
		h.refuse(w, r, err, "listing")
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns one incident with its evidence, narrative and prompt.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.Store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.refuse(w, r, err, "reading")
		return
	}
	if in == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, in)
}

// HandleByAlert returns the incident an alert event is part of, which is how
// the alert detail and the on-call inbox show its narrative.
func (h *Handler) HandleByAlert(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.Store.ByAlertEvent(r.Context(), chi.URLParam(r, "eventId"))
	if err != nil {
		h.refuse(w, r, err, "reading by alert")
		return
	}
	if in == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, in)
}

// HandleRefreshEvidence re-assembles an incident's evidence now.
func (h *Handler) HandleRefreshEvidence(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.RefreshEvidence(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.refuse(w, r, err, "refreshing evidence")
		return
	}
	if in == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, in)
}

// HandleNarrate asks the model to explain an incident on a person's behalf.
func (h *Handler) HandleNarrate(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.Narrate(r.Context(), chi.URLParam(r, "id"), caller(r), false)
	if err != nil {
		h.refuse(w, r, err, "narrating")
		return
	}
	if in == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, in)
}

// HandleSummarise asks the model for a post-incident summary.
func (h *Handler) HandleSummarise(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.Summarise(r.Context(), chi.URLParam(r, "id"), caller(r), false)
	if err != nil {
		h.refuse(w, r, err, "summarising")
		return
	}
	if in == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, in)
}

// HandleResolve closes an open incident now, with its timeline.
func (h *Handler) HandleResolve(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.Store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.refuse(w, r, err, "reading")
		return
	}
	if in == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if in.Status != StatusOpen {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "not_open"})
		return
	}
	out, err := h.svc.ResolveNow(r.Context(), in, caller(r))
	if err != nil {
		h.refuse(w, r, err, "resolving")
		return
	}
	response.OK(w, out)
}

func (h *Handler) settingsView(r *http.Request, envID string) (*SettingsView, error) {
	st, err := h.svc.Store.Settings(r.Context(), envID)
	if err != nil {
		return nil, err
	}
	view := &SettingsView{Settings: st}
	if h.svc.Model != nil {
		if info, err := h.svc.Model.Info(r.Context()); err == nil {
			view.ModelConfigured, view.Model = true, &info
		}
	}
	return view, nil
}

// HandleGetSettings returns an environment's narration settings and where its
// evidence would be sent.
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	env := response.ParseEnvID(r)
	if env == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	view, err := h.settingsView(r, env)
	if err != nil {
		h.refuse(w, r, err, "reading settings")
		return
	}
	response.OK(w, view)
}

// HandleSaveSettings changes an environment's narration consent and limits.
// Opting in is audited as a warning, because from then on evidence can leave.
func (h *Handler) HandleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in SettingsInput
	if err := response.DecodeBody(r, &in); err != nil || in.EnvID == "" || in.HourlyCap < 1 || in.HourlyCap > 100 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	prev, err := h.svc.Store.Settings(r.Context(), in.EnvID)
	if err != nil {
		h.refuse(w, r, err, "reading settings")
		return
	}
	if _, err := h.svc.Store.SaveSettings(r.Context(), Settings{EnvID: in.EnvID, NarrateOptIn: in.NarrateOptIn,
		Proactive: in.Proactive, HourlyCap: in.HourlyCap}, caller(r)); err != nil {
		h.refuse(w, r, err, "saving settings")
		return
	}
	view, err := h.settingsView(r, in.EnvID)
	if err != nil {
		h.refuse(w, r, err, "reading settings")
		return
	}
	if h.app.AuditLog != nil {
		severity := "info"
		if in.NarrateOptIn && !prev.NarrateOptIn {
			severity = "warning"
		}
		provider := ""
		if view.Model != nil {
			provider = view.Model.Provider + "/" + view.Model.Model
		}
		h.app.AuditLog.Log(r, coreAudit.Entry{
			Action: "incident.settings.updated", Severity: severity, EntityType: "incident_settings", EntityID: in.EnvID,
			EnvironmentID: in.EnvID, Details: jsonText(map[string]any{
				"narrateOptIn": in.NarrateOptIn, "proactive": view.Proactive, "hourlyCap": in.HourlyCap, "model": provider,
			}),
		})
	}
	response.OK(w, view)
}

// HandleFindings lists behavioural findings.
func (h *Handler) HandleFindings(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", "open", "dismissed", "cleared":
	default:
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	items, err := h.svc.Store.Findings(r.Context(), response.ParseEnvID(r), status)
	if err != nil {
		h.refuse(w, r, err, "listing findings")
		return
	}
	response.OK(w, items)
}

// HandleExplainFinding has the model put a finding's numbers into words.
func (h *Handler) HandleExplainFinding(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.ExplainFinding(r.Context(), chi.URLParam(r, "fid"), caller(r))
	if err != nil {
		h.refuse(w, r, err, "explaining a finding")
		return
	}
	if f == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, f)
}

// HandleDismissFinding hides a finding until it clears and is found again.
func (h *Handler) HandleDismissFinding(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "fid")
	f, err := h.svc.Store.Finding(r.Context(), id)
	if err != nil {
		h.refuse(w, r, err, "reading a finding")
		return
	}
	if f == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err := h.svc.Store.DismissFinding(r.Context(), id); err != nil {
		h.refuse(w, r, err, "dismissing a finding")
		return
	}
	if h.app.AuditLog != nil {
		h.app.AuditLog.Log(r, coreAudit.Entry{Action: "incident.finding.dismissed", Severity: "info", EntityType: "behaviour_finding",
			EntityID: id, EntityName: f.SubjectName, EnvironmentID: f.EnvID})
	}
	f.Status = "dismissed"
	response.OK(w, f)
}

// HandleSources lists the plugins that raise incidents, with the labels of their
// subject types and signals, so the screens can name what a plugin reported.
func (h *Handler) HandleSources(w http.ResponseWriter, r *http.Request) {
	response.OK(w, Sources())
}

// The rbac.PinEnvToResource resolvers for the incident, by-alert and finding routes.
func (h *Handler) incidentEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.svc.Store.IncidentEnvironment(r.Context(), chi.URLParam(r, "id")))
}

func (h *Handler) alertIncidentEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.svc.Store.AlertIncidentEnvironment(r.Context(), chi.URLParam(r, "eventId")))
}

func (h *Handler) findingEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.svc.Store.FindingEnvironment(r.Context(), chi.URLParam(r, "fid")))
}

func (h *Handler) logResolve(envID string, found bool, err error) (string, bool, error) {
	if err != nil {
		h.app.Logger.Error("incidents: resolving the row's environment", "error", err)
	}
	return envID, found, err
}
