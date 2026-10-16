// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HandleListLocations serves GET /api/compute/locations.
func (h *Handler) HandleListLocations(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.ListLocations(r.Context())
	if err != nil {
		h.respondErr(w, r, "list locations", err)
		return
	}
	ok(w, view)
}

// HandleGetLocations serves GET /api/compute/locations/{envId}.
func (h *Handler) HandleGetLocations(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.LocationsFor(r.Context(), chi.URLParam(r, "envId"))
	if err != nil {
		h.respondErr(w, r, "get locations", err)
		return
	}
	ok(w, view)
}

// HandleSaveLocations serves PUT /api/compute/locations/{envId}.
func (h *Handler) HandleSaveLocations(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := chi.URLParam(r, "envId")

	var req Locations
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if err := req.Validate(); err != nil {
		failErr(w, r, http.StatusBadRequest, codeInvalidPath, err)
		return
	}

	view, err := h.svc.SaveLocations(r.Context(), envID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidLocation) {
			failErr(w, r, http.StatusBadRequest, codeInvalidPath, err)
			return
		}
		h.respondErr(w, r, "save locations", err)
		return
	}

	h.emit(userID, "plugin.compute.locations.updated", map[string]any{
		"environmentId": envID,
		"vmPath":        view.Effective.VMPath,
		"diskPath":      view.Effective.DiskPath,
		"imagePath":     view.Effective.ImagePath,
		"applied":       view.Effective.Applied,
	})

	// Saved either way; the code says whether the host took it, because a
	// refusal is not a failed request and the retry button is the fix.
	if view.Effective.Applied {
		okData(w, view, msgLocationsApplied)
		return
	}
	okData(w, view, msgLocationsSaved)
}

// HandleSaveGlobalLocations serves PUT /api/compute/locations.
//
// The global row is not pushed to any host on save: it is a default, and it
// reaches a host the next time that host's own locations are applied.
func (h *Handler) HandleSaveGlobalLocations(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req Locations
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	view, err := h.svc.SaveGlobalLocations(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidLocation) {
			failErr(w, r, http.StatusBadRequest, codeInvalidPath, err)
			return
		}
		h.respondErr(w, r, "save global locations", err)
		return
	}

	h.emit(userID, "plugin.compute.locations.updated", map[string]any{
		"scopeId":   globalLocationScope,
		"vmPath":    view.Global.VMPath,
		"diskPath":  view.Global.DiskPath,
		"imagePath": view.Global.ImagePath,
	})
	okData(w, view, msgLocationsSaved)
}

// HandleApplyLocations serves POST /api/compute/locations/{envId}/apply.
//
// The retry button: re-push what is stored to a host that was offline, or that
// refused because a guest was still running on the old location.
func (h *Handler) HandleApplyLocations(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := chi.URLParam(r, "envId")

	view, err := h.svc.ApplyLocations(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "apply locations", err)
		return
	}

	h.emit(userID, "plugin.compute.locations.applied", map[string]any{
		"environmentId": envID,
		"applied":       view.Effective.Applied,
	})
	if view.Effective.Applied {
		okData(w, view, msgLocationsApplied)
		return
	}
	okData(w, view, msgLocationsSaved)
}

// HandleRemoveLocations serves DELETE /api/compute/locations/{envId}.
func (h *Handler) HandleRemoveLocations(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := chi.URLParam(r, "envId")

	view, err := h.svc.RemoveLocations(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "remove locations", err)
		return
	}

	h.emit(userID, "plugin.compute.locations.removed", map[string]any{
		"environmentId": envID,
	})
	okData(w, view, msgLocationsRemoved)
}
