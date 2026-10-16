// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Display profile codes. The plugin's own bundle owns the sentences.
const (
	codeDisplayProfileNotFound     = "err.vm.display_profile_not_found"
	codeDisplayProfileBuiltin      = "err.vm.display_profile_builtin"
	codeDisplayProfileInUse        = "err.vm.display_profile_in_use"
	codeDisplayProfileNameTaken    = "err.vm.display_profile_name_taken"
	codeDisplayProfileNameRequired = "err.vm.display_profile_name_required"
	codeDisplayProfileNameTooLong  = "err.vm.display_profile_name_too_long"
	codeDisplayProfileOption       = "err.vm.display_profile_option_invalid"
	codeDisplayProfileUnknown      = "err.vm.display_profile_unknown"

	msgDisplayProfileCreated = "msg.vm.display_profile_created"
	msgDisplayProfileUpdated = "msg.vm.display_profile_updated"
	msgDisplayProfileDeleted = "msg.vm.display_profile_deleted"
)

// displayProfileInUse is the 409 body: the count travels as data so the CLI and
// the dialog can both say how many guests are in the way.
type displayProfileInUse struct {
	InUse int `json:"inUse"`
}

// respondDisplayProfileErr maps the profile errors, and hands anything else to
// respondErr.
func (h *Handler) respondDisplayProfileErr(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var inUse *DisplayProfileInUseError
	switch {
	case errors.As(err, &inUse):
		env := failEnvelope(r, codeDisplayProfileInUse, fmt.Sprintf("In use by %d virtual machine(s).", inUse.Count), err)
		env.Data = displayProfileInUse{InUse: inUse.Count}
		writeJSON(w, http.StatusConflict, env)
	case errors.Is(err, ErrDisplayProfileNotFound):
		failErr(w, r, http.StatusNotFound, codeDisplayProfileNotFound, err)
	case errors.Is(err, ErrDisplayProfileBuiltin):
		failErr(w, r, http.StatusConflict, codeDisplayProfileBuiltin, err)
	case errors.Is(err, ErrDisplayProfileNameTaken):
		failErr(w, r, http.StatusConflict, codeDisplayProfileNameTaken, err)
	default:
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected display profile", "operation", operation, "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.logger.Error("kvm: display profile operation failed", "operation", operation, "error", err)
		failErr(w, r, http.StatusInternalServerError, codeCommandFailed, err)
	}
}

// HandleListDisplayProfiles serves GET /api/vms/display-profiles.
func (h *Handler) HandleListDisplayProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.svc.ListDisplayProfiles(r.Context())
	if err != nil {
		h.respondDisplayProfileErr(w, r, "list display profiles", err)
		return
	}
	ok(w, profiles)
}

// HandleGetDisplayProfile serves GET /api/vms/display-profiles/{profileId}.
func (h *Handler) HandleGetDisplayProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.svc.DisplayProfile(r.Context(), chi.URLParam(r, "profileId"))
	if err != nil {
		h.respondDisplayProfileErr(w, r, "read display profile", err)
		return
	}
	ok(w, profile)
}

// HandleCreateDisplayProfile serves POST /api/vms/display-profiles.
func (h *Handler) HandleCreateDisplayProfile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	var req DisplayProfileRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	profile, err := h.svc.CreateDisplayProfile(r.Context(), req)
	if err != nil {
		h.respondDisplayProfileErr(w, r, "create display profile", err)
		return
	}
	h.emit(userID, "plugin.kvm.display_profile.created", displayProfileAuditFields(profile))
	created(w, profile, msgDisplayProfileCreated)
}

// HandleUpdateDisplayProfile serves PUT /api/vms/display-profiles/{profileId}.
func (h *Handler) HandleUpdateDisplayProfile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	var req DisplayProfileRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	profile, err := h.svc.UpdateDisplayProfile(r.Context(), chi.URLParam(r, "profileId"), req)
	if err != nil {
		h.respondDisplayProfileErr(w, r, "update display profile", err)
		return
	}
	h.emit(userID, "plugin.kvm.display_profile.updated", displayProfileAuditFields(profile))
	okData(w, profile, msgDisplayProfileUpdated)
}

// HandleDuplicateDisplayProfile serves POST /api/vms/display-profiles/{profileId}/duplicate.
func (h *Handler) HandleDuplicateDisplayProfile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	sourceID := chi.URLParam(r, "profileId")
	profile, err := h.svc.DuplicateDisplayProfile(r.Context(), sourceID)
	if err != nil {
		h.respondDisplayProfileErr(w, r, "duplicate display profile", err)
		return
	}
	fields := displayProfileAuditFields(profile)
	fields["sourceProfileId"] = sourceID
	h.emit(userID, "plugin.kvm.display_profile.duplicated", fields)
	created(w, profile, msgDisplayProfileCreated)
}

// HandleDeleteDisplayProfile serves DELETE /api/vms/display-profiles/{profileId}.
//
// ?reassign=true is the operator's explicit consent to move the guests still
// using the profile to Fast LAN; without it a profile in use is refused.
func (h *Handler) HandleDeleteDisplayProfile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "profileId")
	reassign := r.URL.Query().Get("reassign") == "true"
	moved, err := h.svc.DeleteDisplayProfile(r.Context(), id, reassign)
	if err != nil {
		h.respondDisplayProfileErr(w, r, "delete display profile", err)
		return
	}
	h.emit(userID, "plugin.kvm.display_profile.deleted", map[string]any{
		"profileId": id, "reassigned": moved,
	})
	okMsg(w, msgDisplayProfileDeleted)
}

// displayProfileAuditFields records what a profile now does, so the trail shows
// which options a guest was about to boot with.
func displayProfileAuditFields(profile DisplayProfile) map[string]any {
	return map[string]any{
		"profileId": profile.ID, "name": profile.Name,
		"imageCompression": profile.ImageCompression, "jpegWanCompression": profile.JPEGWANCompression,
		"zlibGlzWanCompression": profile.ZlibGLZWANCompression, "streamingVideo": profile.StreamingVideo,
		"playbackCompression": profile.PlaybackCompression, "agentMouse": profile.AgentMouse,
	}
}
