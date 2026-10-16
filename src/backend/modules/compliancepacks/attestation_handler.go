// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// registerAttestationRoutes mounts the sign-off record: create and read, and
// deliberately nothing else. A wrong attestation is superseded by a newer one,
// never edited, so the history an auditor reads cannot be rewritten.
func registerAttestationRoutes(r *apispec.Router, h *handler, view, attest func(http.Handler) http.Handler) {
	r.Route("/compliance/attestations", func(r *apispec.Router) {
		r.With(view).Get("/", h.handleListAttestations,
			apispec.Query("framework", "Only attestations for this framework; empty means all"),
			apispec.Paginated())
		r.With(attest).Post("/", h.handleCreateAttestation,
			apispec.Request(AttestationInput{}),
			apispec.Response(Attestation{}))
	})
}

func (h *handler) handleListAttestations(w http.ResponseWriter, r *http.Request) {
	framework := strings.TrimSpace(r.URL.Query().Get("framework"))
	if framework != "" && !attestable(framework) {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "unknown_framework"})
		return
	}
	page, perPage := response.ParsePagination(r)
	items, total, err := h.svc.ListAttestations(r.Context(), framework, perPage, (page-1)*perPage)
	if err != nil {
		h.app.Logger.Error("compliance attestations: listing failed", "framework", framework, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.Paginated(w, items, int64(total), page, perPage)
}

// handleCreateAttestation records a sign-off attributed to the authenticated user.
// The audit row is written with the real request so its actor column is the
// person who signed, never NULL with the name buried in free text.
func (h *handler) handleCreateAttestation(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in AttestationInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	created, err := h.svc.Attest(r.Context(), user.ID, in)
	switch {
	case errors.Is(err, ErrAttestationFramework):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "unknown_framework"})
		return
	case errors.Is(err, ErrAttestationNote):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "note_required"})
		return
	case errors.Is(err, ErrAttestationScope):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "scope_too_long"})
		return
	case errors.Is(err, ErrAttestationRun):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "run_mismatch"})
		return
	case errors.Is(err, ErrAttestationRunOpen):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "run_not_finished"})
		return
	case errors.Is(err, ErrAttestationActor):
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	case err != nil:
		h.app.Logger.Error("compliance attestations: recording failed", "framework", in.Framework, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	details, err := json.Marshal(map[string]string{
		"framework": created.Framework, "runId": created.RunID, "scope": created.Scope, "note": created.Note,
	})
	if err != nil {
		h.app.Logger.Error("compliance attestations: marshalling audit details failed", "error", err)
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "compliance.attestation.create",
		Severity:   "info",
		EntityType: "compliance_attestation",
		EntityID:   created.ID,
		EntityName: created.Framework,
		Details:    string(details),
	})
	response.Created(w, created)
}
