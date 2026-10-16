// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package secrets serves the secret vault endpoints: storage, rotation, reveal, generation and applying values to running resources.
package secrets

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	coresecrets "github.com/therealmcsparrow/mcharbor/core/secrets"
)

// HandleImpact reports what a secret is used by and what still runs an older
// version of it.
//
// Read-only and cheap, so the dialog and the rotation banner both poll it
// rather than each deriving staleness from the raw reference list.
// GET /api/secrets/{id}/impact
func (h *Handler) HandleImpact(w http.ResponseWriter, r *http.Request) {
	impact, err := h.service.Impact(r.Context(), h.app.SecretAppliers, chi.URLParam(r, "id"))
	if err != nil {
		h.respondServiceError(w, r, "impact", err, i18n.ErrSecretImpactFailed)
		return
	}
	response.OK(w, impact)
}

// HandleApply recreates containers and redeploys stacks so they run with the
// secret's current value.
//
// An empty `resources` means every stale resource. Gated on `secrets.apply`
// rather than `secrets.manage`, because this is the call that takes a
// production stack down for the length of a deploy.
// POST /api/secrets/{id}/apply
func (h *Handler) HandleApply(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.ApplyRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	for _, target := range req.Resources {
		if target.ResourceType == "" || target.ResourceID == "" {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
	}

	id := chi.URLParam(r, "id")
	sec, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.respondServiceError(w, r, "apply", err, i18n.ErrSecretApplyFailed)
		return
	}

	results, err := h.service.Apply(r.Context(), h.app.SecretAppliers, id, req.Resources)
	if err != nil {
		h.respondServiceError(w, r, "apply", err, i18n.ErrSecretApplyFailed)
		return
	}

	h.logApplyResults(sec, results)
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "apply_secret",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    applyAuditDetails(sec.Version, results),
	})

	response.OK(w, results)
}

// HandlePruneReferences deletes the reference rows for resources that no longer
// exist, or for the ones named in the body.
//
// Gated on `secrets.manage` rather than `secrets.apply`: this touches
// bookkeeping, not running workloads. It is a deliberate action and never a
// side effect of reading the impact, so a reference that should not have gone
// stale stays visible until somebody decides otherwise.
// POST /api/secrets/{id}/references/prune
func (h *Handler) HandlePruneReferences(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.ApplyRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	id := chi.URLParam(r, "id")
	sec, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.respondServiceError(w, r, "prune references", err, i18n.ErrSecretPruneFailed)
		return
	}

	pruned, err := h.service.PruneReferences(r.Context(), h.app.SecretAppliers, id, req.Resources)
	if err != nil {
		h.respondServiceError(w, r, "prune references", err, i18n.ErrSecretPruneFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "prune_secret_references",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "pruned=" + strconv.Itoa(pruned),
	})

	response.OK(w, map[string]int{"pruned": pruned})
}

// applyAuditDetails renders the run as one line: the version propagated and
// how each resource fared. The audit entry is the only durable record of which
// resources a rotation restarted, so it names them rather than counting them.
func applyAuditDetails(version int, results []coresecrets.ApplyResult) string {
	details := "version=" + strconv.Itoa(version)
	for _, result := range results {
		details += " " + result.ResourceType + ":" + result.ResourceName + "=" + result.Status
	}
	return details
}

// logApplyResults writes the reason a resource failed to the server log, where
// it belongs: the detail carries Docker and compose output, which is exactly
// the class of text that must not reach a browser.
func (h *Handler) logApplyResults(sec *coresecrets.Secret, results []coresecrets.ApplyResult) {
	for _, result := range results {
		if result.Detail == "" {
			continue
		}
		h.app.Logger.Error("secrets: applying a rotation failed",
			"secret", sec.Name,
			"resource_type", result.ResourceType,
			"resource", result.ResourceName,
			"status", result.Status,
			"detail", result.Detail)
	}
}

// autoApply propagates a rotation for a secret that asked to propagate its own.
//
// It runs on the application context rather than the request's, in its own
// goroutine, and that is the point: applying can mean forty container
// recreates, and holding the rotate request open for the length of them would
// time out in the browser long before the work finished. The operator watches
// progress through the impact endpoint, which is what the dialog polls.
//
// The goroutine is bounded by the same per-resource timeout the synchronous
// path uses and by the application context, so a shutdown stops it.
func (h *Handler) autoApply(reqCtx context.Context, sec *coresecrets.Secret) { //nolint:contextcheck // the rotation outlives the request and must stop on application shutdown, so it is rooted on the application context
	enabled, err := h.service.AutoApplyEnabled(reqCtx, sec.ID)
	if err != nil {
		h.app.Logger.Warn("secrets: reading the auto-apply flag failed",
			"secret", sec.Name, "error", err)
		return
	}
	if !enabled {
		return
	}

	ctx := h.app.ContextOrBackground()
	go func() {
		results, err := h.service.Apply(ctx, h.app.SecretAppliers, sec.ID, nil)
		if err != nil {
			h.app.Logger.Error("secrets: auto-applying a rotation failed",
				"secret", sec.Name, "error", err)
			return
		}
		h.logApplyResults(sec, results)
		h.app.Logger.Info("secrets: rotation auto-applied",
			"secret", sec.Name, "resources", len(results))
	}()
}
