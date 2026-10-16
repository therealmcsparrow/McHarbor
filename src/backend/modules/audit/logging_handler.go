// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

const (
	auditActionForwarderSaved   = "audit.forwarder.saved"
	auditActionForwarderTested  = "audit.forwarder.tested"
	auditActionForwarderDeleted = "audit.forwarder.deleted"
)

// SetPluginRuntime injects the loaded plugin runtime so the logging
// service can reach plugin-registered forwarders.
func (h *Handler) SetPluginRuntime(runtime any) {
	if h.logging != nil {
		h.logging.SetPluginRuntime(runtime)
	}
}

// HandleForwarderAdapters lists the installed adapter types.
//
// Separate from the instance list because the Add picker asks a
// different question than the tab's list does: "what transports can I
// use" versus "what am I currently shipping to". One adapter backs any
// number of instances.
func (h *Handler) HandleForwarderAdapters(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.logging.Adapters())
}

// HandleForwarders lists every configured forwarder instance with its
// stored config merged in.
//
// The response carries no secret values — only a per-field boolean
// saying one is stored. See LoggingService.buildInfo.
func (h *Handler) HandleForwarders(w http.ResponseWriter, r *http.Request) {
	items, err := h.logging.Forwarders(r.Context())
	if err != nil {
		h.app.Logger.Error("audit: listing log forwarders failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAuditForwarderListFailed, err)
		return
	}
	response.OK(w, items)
}

// saveForwarderRequest is the Logging tab's form submission. Config
// carries the adapter's declared ConfigSchema keys; the host does not
// know or validate their meaning beyond the declared type.
type saveForwarderRequest struct {
	Name string `json:"name"`
	// ForwarderType is read on create only — see SaveForwarderInput.
	ForwarderType string            `json:"forwarderType"`
	Enabled       bool              `json:"enabled"`
	Config        map[string]string `json:"config"`
}

func (req saveForwarderRequest) input() SaveForwarderInput {
	return SaveForwarderInput(req)
}

// writeSaveError maps the shared create/update validation failures.
// Returns true when it handled the error.
func (h *Handler) writeSaveError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, ErrForwarderUnknown):
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderUnknown)
	case errors.Is(err, ErrForwarderNotConfigured):
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderNotConfigured)
	case errors.Is(err, ErrForwarderNameRequired):
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderNameRequired)
	case errors.Is(err, ErrForwarderFieldRequired):
		// The operator left a field the adapter declared required
		// empty. Their form to fix, not a server fault — and the
		// response names no field value, only the shape of the
		// mistake.
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderFieldRequired)
	default:
		return false
	}
	return true
}

// HandleCreateForwarder adds one forwarder instance.
func (h *Handler) HandleCreateForwarder(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req saveForwarderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(req.ForwarderType) == "" {
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderTypeRequired)
		return
	}

	id, err := h.logging.Create(r.Context(), req.input())
	if h.writeSaveError(w, r, err) {
		return
	}
	if err != nil {
		h.app.Logger.Error("audit: creating log forwarder failed",
			"forwarder", req.ForwarderType, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAuditForwarderCreateFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionForwarderSaved,
		Severity:   "warning",
		EntityType: "audit_log_forwarder",
		EntityID:   id,
		EntityName: strings.TrimSpace(req.Name),
		Details:    req.ForwarderType,
	})

	response.Created(w, map[string]string{"id": id})
}

// HandleSaveForwarder rewrites one forwarder instance.
func (h *Handler) HandleSaveForwarder(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderTypeRequired)
		return
	}

	var req saveForwarderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	err := h.logging.Update(r.Context(), id, req.input())
	if h.writeSaveError(w, r, err) {
		return
	}
	if err != nil {
		h.app.Logger.Error("audit: saving log forwarder failed",
			"forwarder", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAuditForwarderSaveFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionForwarderSaved,
		Severity:   "warning",
		EntityType: "audit_log_forwarder",
		EntityID:   id,
		EntityName: strings.TrimSpace(req.Name),
	})

	response.OKMsg(w, r, i18n.MsgAuditForwarderSaved)
}

// HandleDeleteForwarder removes one forwarder instance.
//
// The plugin stays installed and its adapter stays registered — what
// goes is this destination. Uninstalling the plugin is a separate
// action on a separate page.
func (h *Handler) HandleDeleteForwarder(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderTypeRequired)
		return
	}

	// Read before deleting so the audit entry names what was removed.
	// An entry saying only "id cvv0f2..." is useless six months later,
	// which is exactly when it gets read.
	info, err := h.logging.Forwarder(r.Context(), id)
	if errors.Is(err, ErrForwarderNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderNotConfigured)
		return
	}
	if err != nil {
		h.app.Logger.Error("audit: reading log forwarder before delete failed",
			"forwarder", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAuditForwarderDeleteFailed, err)
		return
	}

	err = h.logging.Delete(r.Context(), id)
	if errors.Is(err, ErrForwarderNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderNotConfigured)
		return
	}
	if err != nil {
		h.app.Logger.Error("audit: deleting log forwarder failed",
			"forwarder", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAuditForwarderDeleteFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionForwarderDeleted,
		Severity:   "warning",
		EntityType: "audit_log_forwarder",
		EntityID:   id,
		EntityName: info.Name,
		Details:    info.ForwarderType,
	})

	response.OKMsg(w, r, i18n.MsgAuditForwarderDeleted)
}

// HandleTestForwarder asks the adapter to hit the wire.
//
// A transport failure is reported as a 400 with a generic code, not a
// 500: the request was well-formed and the server worked correctly —
// it is the operator's endpoint, credential, or firewall that is
// wrong, and that is theirs to fix. The real error is logged
// server-side with the forwarder id for support.
func (h *Handler) HandleTestForwarder(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderTypeRequired)
		return
	}

	err := h.logging.Test(r.Context(), id)
	if errors.Is(err, ErrForwarderNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderNotConfigured)
		return
	}
	if errors.Is(err, ErrForwarderUnknown) {
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderUnknown)
		return
	}

	status := "success"
	if err != nil {
		status = "failed"
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionForwarderTested,
		Severity:   "info",
		EntityType: "audit_log_forwarder",
		EntityID:   id,
		EntityName: id,
		Details:    status,
	})

	if err != nil {
		h.app.Logger.Error("audit: log forwarder test failed",
			"forwarder", id, "error", err)
		response.BadRequestCause(w, r, i18n.ErrAuditForwarderTestFailed, err)
		return
	}
	response.OKMsg(w, r, i18n.MsgAuditForwarderTested)
}

// HandleForwarderHistory returns this instance's recent delivery
// attempts.
func (h *Handler) HandleForwarderHistory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrAuditForwarderTypeRequired)
		return
	}
	items, err := h.logging.History(r.Context(), id)
	if errors.Is(err, ErrForwarderNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderNotConfigured)
		return
	}
	if errors.Is(err, ErrForwarderUnknown) {
		response.NotFoundCode(w, r, i18n.ErrAuditForwarderUnknown)
		return
	}
	if err != nil {
		h.app.Logger.Error("audit: log forwarder history failed",
			"forwarder", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAuditForwarderHistoryFailed, err)
		return
	}
	response.OK(w, items)
}
