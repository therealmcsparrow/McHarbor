// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// RejectInput is the body of a rejection.
type RejectInput struct {
	Note string `json:"note"`
}

// ConfirmPassesInput is the body that changes the debounce.
type ConfirmPassesInput struct {
	ConfirmPasses int `json:"confirmPasses"`
}

// EnforcementInput is the body that changes the enforcement switch and limits.
type EnforcementInput struct {
	EnforceEnabled    bool `json:"enforceEnabled"`
	MaxActionsPerPass int  `json:"maxActionsPerPass"`
	CooldownSeconds   int  `json:"cooldownSeconds"`
	FailureThreshold  int  `json:"failureThreshold"`
}

// ProposalOutcome is what an approval returns: the proposal that was applied,
// or, when reality moved first, the fresh one that replaced it.
type ProposalOutcome struct {
	Proposal *Proposal `json:"proposal"`
	Stale    bool      `json:"stale"`
}

func (h *Handler) audit(r *http.Request, action, severity string, ds *DesiredState, details map[string]any) {
	if h.app.AuditLog == nil {
		return
	}
	raw, err := json.Marshal(details)
	if err != nil {
		h.app.Logger.Error("reconciler: encoding audit details failed", "error", err)
		raw = []byte("{}")
	}
	e := coreAudit.Entry{Action: action, Severity: severity, EntityType: "desired_state", Details: string(raw)}
	if ds != nil {
		e.EntityID, e.EntityName, e.EnvironmentID = ds.ID, ds.Name, ds.EnvID
	}
	h.app.AuditLog.Log(r, e)
}

// refuse answers a responder error with the reason code the UI translates.
func (h *Handler) refuse(w http.ResponseWriter, r *http.Request, err error, logMsg string) {
	switch reason := reasonOf(err); reason {
	case "unknown":
		h.app.Logger.Warn(logMsg, "error", err)
		response.ErrCodeWithData(w, r, http.StatusServiceUnavailable, i18n.ErrReconcilerDockerFailed, map[string]string{"reason": reason})
	case "":
		h.app.Logger.Error(logMsg, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
	default:
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerFailed, map[string]string{"reason": reason})
	}
}

// HandleListProposals lists plans, newest first, by status and environment.
func (h *Handler) HandleListProposals(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", ProposalPending, ProposalApplying, ProposalApplied, ProposalFailed, ProposalRejected, ProposalSuperseded:
	default:
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	page, perPage := response.ParsePagination(r)
	items, total, err := h.responder.Proposals.List(r.Context(), status, response.ParseEnvID(r), page, perPage)
	if err != nil {
		h.app.Logger.Error("reconciler: listing proposals failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerListFailed, err)
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// HandleGetProposal returns one plan.
func (h *Handler) HandleGetProposal(w http.ResponseWriter, r *http.Request) {
	p, err := h.responder.Proposals.Get(r.Context(), chi.URLParam(r, "pid"))
	if err != nil {
		h.app.Logger.Error("reconciler: reading proposal failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	// The route pins ?env= to the proposal's own environment; answering 404 on a mismatch
	// keeps a direct or mis-wired call from reading another environment's plan.
	if p == nil || p.EnvID != response.ParseEnvID(r) {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}
	response.OK(w, p)
}

// HandlePropose writes a plan for a desired state's drift as it is right now.
func (h *Handler) HandlePropose(w http.ResponseWriter, r *http.Request) {
	ds, ok := h.desired(w, r)
	if !ok {
		return
	}
	p, err := h.responder.ProposeNow(r.Context(), ds)
	if err != nil {
		h.refuse(w, r, err, "reconciler: proposing failed")
		return
	}
	h.audit(r, "reconciler.proposal.created", "info", ds, map[string]any{"proposalId": p.ID, "steps": p.Steps})
	response.OK(w, p)
}

// HandleApprove applies a plan on the caller's behalf, after checking reality
// has not moved since it was written.
func (h *Handler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	pid := chi.URLParam(r, "pid")
	p, err := h.responder.Approve(r.Context(), pid, user.ID)
	if errors.Is(err, ErrStale) {
		h.audit(r, "reconciler.proposal.stale", "info", nil, map[string]any{"proposalId": pid})
		response.OK(w, ProposalOutcome{Proposal: p, Stale: true})
		return
	}
	if err != nil {
		h.refuse(w, r, err, "reconciler: approving failed")
		return
	}
	if p == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}
	ds, dsErr := h.service.ByID(r.Context(), p.DesiredStateID)
	if dsErr != nil {
		h.app.Logger.Error("reconciler: load desired state for audit failed", "error", dsErr, "id", p.DesiredStateID)
	}
	severity := "warning"
	if p.Status == ProposalFailed {
		severity = "error"
	}
	h.audit(r, "reconciler.proposal."+p.Status, severity, ds, map[string]any{
		"proposalId": p.ID, "diffs": reportDiffs(p.Report), "steps": p.Steps, "results": p.Results, "error": p.Error,
	})
	response.OK(w, ProposalOutcome{Proposal: p})
}

func reportDiffs(r *DriftReport) []DriftDiff {
	if r == nil {
		return nil
	}
	return r.Diffs
}

// HandleReject closes a plan without acting.
func (h *Handler) HandleReject(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in RejectInput
	if r.ContentLength > 0 {
		if err := response.DecodeBody(r, &in); err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
	}
	if len(in.Note) > 1000 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	p, err := h.responder.Reject(r.Context(), chi.URLParam(r, "pid"), user.ID, in.Note)
	if err != nil {
		h.refuse(w, r, err, "reconciler: rejecting failed")
		return
	}
	ds, dsErr := h.service.ByID(r.Context(), p.DesiredStateID)
	if dsErr != nil {
		h.app.Logger.Error("reconciler: load desired state for audit failed", "error", dsErr, "id", p.DesiredStateID)
	}
	h.audit(r, "reconciler.proposal.rejected", "info", ds, map[string]any{"proposalId": p.ID, "note": in.Note})
	response.OK(w, p)
}

// HandleEnforce opts one desired state into enforcement. It is its own route
// so that granting enforcement is a separate permission from editing intent.
func (h *Handler) HandleEnforce(w http.ResponseWriter, r *http.Request) {
	ds, ok := h.desired(w, r)
	if !ok {
		return
	}
	if err := h.service.SetMode(r.Context(), ds.ID, ModeEnforce); err != nil {
		h.app.Logger.Error("reconciler: enabling enforcement failed", "error", err, "id", ds.ID)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	h.audit(r, "reconciler.enforce.enabled", "warning", ds, map[string]any{"previousMode": ds.Mode})
	updated, err := h.service.ByID(r.Context(), ds.ID)
	if err != nil || updated == nil {
		response.InternalErrorCode(w, r, i18n.ErrReconcilerUpdateFailed)
		return
	}
	response.OK(w, updated)
}

// HandleGetSettings returns the debounce, enforcement limits and breaker state.
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.responder.Settings.Load(r.Context())
	if err != nil {
		h.app.Logger.Error("reconciler: reading settings failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	response.OK(w, st)
}

// HandleSetConfirmPasses changes how many passes confirm drift.
func (h *Handler) HandleSetConfirmPasses(w http.ResponseWriter, r *http.Request) {
	var in ConfirmPassesInput
	if err := response.DecodeBody(r, &in); err != nil || in.ConfirmPasses < MinConfirmPasses || in.ConfirmPasses > MaxConfirmPasses {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.responder.Settings.SetConfirmPasses(r.Context(), in.ConfirmPasses); err != nil {
		h.app.Logger.Error("reconciler: saving settings failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	h.audit(r, "reconciler.settings.updated", "info", nil, map[string]any{"confirmPasses": in.ConfirmPasses})
	h.HandleGetSettings(w, r)
}

// HandleSetEnforcement changes the enforcement switch and its limits.
func (h *Handler) HandleSetEnforcement(w http.ResponseWriter, r *http.Request) {
	var in EnforcementInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	st, err := h.responder.Settings.Load(r.Context())
	if err != nil {
		h.app.Logger.Error("reconciler: reading settings failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	st.EnforceEnabled, st.MaxActionsPerPass, st.CooldownSeconds, st.FailureThreshold =
		in.EnforceEnabled, in.MaxActionsPerPass, in.CooldownSeconds, in.FailureThreshold
	if err := st.Validate(); err != nil {
		h.app.Logger.Info("reconciler: input refused", "error", err)
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": validationReason(err)})
		return
	}
	if err := h.responder.Settings.SetEnforcement(r.Context(), st); err != nil {
		h.app.Logger.Error("reconciler: saving enforcement failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	h.audit(r, "reconciler.enforcement.updated", "warning", nil, map[string]any{
		"enforceEnabled": in.EnforceEnabled, "maxActionsPerPass": in.MaxActionsPerPass,
		"cooldownSeconds": in.CooldownSeconds, "failureThreshold": in.FailureThreshold,
	})
	h.HandleGetSettings(w, r)
}

// HandleResetBreaker closes the circuit breaker so enforcement resumes.
func (h *Handler) HandleResetBreaker(w http.ResponseWriter, r *http.Request) {
	if err := h.responder.Settings.Reset(r.Context()); err != nil {
		h.app.Logger.Error("reconciler: resetting breaker failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	h.audit(r, "reconciler.enforce.breaker_reset", "warning", nil, map[string]any{})
	h.HandleGetSettings(w, r)
}

// desired loads the {id} desired state or answers 404/500 itself.
func (h *Handler) desired(w http.ResponseWriter, r *http.Request) (*DesiredState, bool) {
	id := chi.URLParam(r, "id")
	ds, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("reconciler: lookup failed", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return nil, false
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return nil, false
	}
	return ds, true
}
