// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

var prClient = &http.Client{Timeout: 30 * time.Second}

// HandleListSources lists the Git sources of desired state.
func (h *Handler) HandleListSources(w http.ResponseWriter, r *http.Request) {
	items, err := h.sources.List(r.Context())
	if err != nil {
		h.app.Logger.Error("reconciler: listing sources failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerListFailed, err)
		return
	}
	response.OK(w, items)
}

// HandleCreateSource adds a Git source and syncs it once, so a bad path or
// document is reported at creation rather than on the next pass.
func (h *Handler) HandleCreateSource(w http.ResponseWriter, r *http.Request) {
	var in SourceInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if !h.app.RBACService.GateOn(w, r, in.EnvID, rbac.PermReconcilerManage) {
		return
	}
	if err := in.Validate(); err != nil {
		h.app.Logger.Info("reconciler: input refused", "error", err)
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": validationReason(err)})
		return
	}
	src, err := h.sources.Create(r.Context(), in)
	if err != nil {
		h.app.Logger.Error("reconciler: creating source failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerCreateFailed, err)
		return
	}
	h.audit(r, "reconciler.source.created", "info", nil, map[string]any{"sourceId": src.ID, "repoId": src.RepoID, "path": src.Path})
	if src.Enabled {
		if _, err := h.sources.Sync(r.Context(), src, true); err != nil {
			h.app.Logger.Warn("reconciler: first sync failed", "source", src.ID, "error", err)
		}
		if fresh, err := h.sources.Get(r.Context(), src.ID); err == nil && fresh != nil {
			src = fresh
		}
	}
	response.Created(w, src)
}

// HandleUpdateSource replaces a source's settings.
func (h *Handler) HandleUpdateSource(w http.ResponseWriter, r *http.Request) {
	var in SourceInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if !h.app.RBACService.GateOn(w, r, in.EnvID, rbac.PermReconcilerManage) {
		return
	}
	if err := in.Validate(); err != nil {
		h.app.Logger.Info("reconciler: input refused", "error", err)
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": validationReason(err)})
		return
	}
	src, err := h.sources.Update(r.Context(), chi.URLParam(r, "sid"), in)
	if err != nil {
		h.app.Logger.Error("reconciler: updating source failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	if src == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}
	h.audit(r, "reconciler.source.updated", "info", nil, map[string]any{"sourceId": src.ID})
	response.OK(w, src)
}

// HandleDeleteSource removes a source and hands its rows back to the API.
func (h *Handler) HandleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sid")
	if err := h.sources.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
			return
		}
		h.app.Logger.Error("reconciler: deleting source failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	h.audit(r, "reconciler.source.deleted", "warning", nil, map[string]any{"sourceId": id})
	response.NoContent(w)
}

// HandleSyncSource syncs a source now, whether or not its branch moved.
// The error is the Git or document problem, recorded on the source for the
// operator; the response carries it in data so the dialog can show it whole.
func (h *Handler) HandleSyncSource(w http.ResponseWriter, r *http.Request) {
	src, err := h.sources.Get(r.Context(), chi.URLParam(r, "sid"))
	if err != nil {
		h.app.Logger.Error("reconciler: reading source failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	if src == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}
	res, err := h.sources.Sync(r.Context(), src, true)
	if err != nil {
		h.app.Logger.Warn("reconciler: sync failed", "source", src.ID, "error", err)
		response.ErrCodeWithData(w, r, http.StatusBadGateway, i18n.ErrReconcilerFailed, map[string]string{"reason": "sync_failed"})
		return
	}
	response.OK(w, res)
}

// HandlePullRequest writes a Git-managed row's drift back to its repository as
// a pull request that changes the intent to match reality.
func (h *Handler) HandlePullRequest(w http.ResponseWriter, r *http.Request) {
	ds, ok := h.desired(w, r)
	if !ok {
		return
	}
	if ds.SourceID == "" {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerFailed, map[string]string{"reason": "not_git_managed"})
		return
	}
	res, err := h.sources.OpenPullRequest(r.Context(), ds, prClient)
	switch {
	case errors.Is(err, ErrNotExpressible):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerFailed, map[string]string{"reason": "not_expressible"})
		return
	case errors.Is(err, ErrNoDrift):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerFailed, map[string]string{"reason": "no_drift"})
		return
	case err != nil:
		h.app.Logger.Warn("reconciler: pull request failed", "id", ds.ID, "error", err)
		response.ErrCodeWithData(w, r, http.StatusBadGateway, i18n.ErrReconcilerFailed, map[string]string{"reason": "git_failed"})
		return
	}
	h.audit(r, "reconciler.drift.pull_request", "info", ds, map[string]any{
		"branch": res.Branch, "url": res.PullRequestURL, "changed": res.Changed, "skipped": res.Skipped,
	})
	response.OK(w, res)
}
