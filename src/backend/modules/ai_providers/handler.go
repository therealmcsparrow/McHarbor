// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_providers

import (
	"context"
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

// Handler holds dependencies for AI provider connection HTTP handlers.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler creates a new AI provider connection handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app: app,
		svc: NewService(app.DB, app.Encryption, app.PluginAdapters),
	}
}

// HandleList returns all AI provider connections.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	items, err := h.svc.List(r.Context())
	if err != nil {
		h.app.Logger.Error("ai providers: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderListFailed, err)
		return
	}
	response.OK(w, items)
}

// HandleGet returns a single AI provider connection.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	conn, err := h.svc.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("ai providers: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderListFailed, err)
		return
	}
	if conn == nil {
		response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
		return
	}
	response.OK(w, conn)
}

// HandleCreate creates a new AI provider connection.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var input CreateAIProviderInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrAIProviderNameRequired)
		return
	}
	if input.ProviderType == "" {
		response.BadRequestCode(w, r, i18n.ErrAIProviderTypeRequired)
		return
	}

	conn, err := h.svc.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidConnection) || errors.Is(err, ErrPluginNotLoaded) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		h.app.Logger.Error("ai providers: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderCreateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.created",
		EntityType: "ai_provider_connection",
		EntityID:   conn.ID,
		EntityName: conn.Name,
	})
	response.Created(w, conn)
}

// HandleUpdate applies partial updates to an AI provider connection.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var input UpdateAIProviderInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	conn, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, ErrInvalidConnection) || errors.Is(err, ErrPluginNotLoaded) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		h.app.Logger.Error("ai providers: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderUpdateFailed, err)
		return
	}
	if conn == nil {
		response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.updated",
		EntityType: "ai_provider_connection",
		EntityID:   conn.ID,
		EntityName: conn.Name,
	})
	response.OK(w, conn)
}

// HandleDelete removes an AI provider connection.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	// Read before the delete: afterwards there is no row left to name.
	name := h.auditConnectionName(r.Context(), id)
	deleted, err := h.svc.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("ai providers: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderRemoveFailed, err)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.deleted",
		EntityType: "ai_provider_connection",
		EntityID:   id,
		EntityName: name,
	})
	response.NoContent(w)
}

// HandleSetDefault marks the given connection as the default, clearing
// any prior default within the same transaction.
func (h *Handler) HandleSetDefault(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.svc.SetDefault(r.Context(), id); err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
			return
		}
		if errors.Is(err, ErrPluginNotLoaded) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		h.app.Logger.Error("ai providers: set default error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderUpdateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.default_set",
		EntityType: "ai_provider_connection",
		EntityID:   id,
		EntityName: h.auditConnectionName(r.Context(), id),
	})
	response.OKMsg(w, r, i18n.MsgAIProviderDefaultSet)
}

// HandleTest sends a test completion through the configured plugin for
// the connection and returns a per-step report (load → decrypt →
// plugin lookup → send). Always 200 OK with the result so the UI can
// render the per-step modal that storage_locations and communications
// already use; the plugin's own failure is captured as the send step's
// detail, not a 4xx. A non-existent connection still returns 404.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	result, err := h.svc.TestConnection(r.Context(), id)
	if err != nil {
		h.auditTest(r, id, "fail")
		if errors.Is(err, ErrConnectionNotFound) {
			response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
			return
		}
		h.app.Logger.Error("ai providers: test error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderTestFailed, err)
		return
	}

	details, detailsErr := json.Marshal(map[string]string{"status": result.OverallStatus})
	if detailsErr != nil {
		h.app.Logger.Error("ai providers: marshal audit details failed", "error", detailsErr, "id", id)
	} else {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "ai_provider.tested",
			Severity:   auditStatusSeverity(result.OverallStatus),
			EntityType: "ai_provider_connection",
			EntityID:   id,
			EntityName: h.auditConnectionName(r.Context(), id),
			Details:    string(details),
		})
	}
	response.OK(w, result)
}

// auditConnectionName resolves a connection id to its name for the audit trail,
// empty when the lookup fails: a deleted connection that appears in the log
// only as an xid is what this is here to prevent.
func (h *Handler) auditConnectionName(ctx context.Context, id string) string {
	conn, err := h.svc.ByID(ctx, id)
	if err != nil || conn == nil {
		return ""
	}
	return conn.Name
}

func (h *Handler) auditTest(r *http.Request, id string, status string) {
	details, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		h.app.Logger.Error("ai providers: marshal audit details failed", "error", err, "id", id)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.tested",
		Severity:   auditStatusSeverity(status),
		EntityType: "ai_provider_connection",
		EntityID:   id,
		EntityName: h.auditConnectionName(r.Context(), id),
		Details:    string(details),
	})
}

func auditStatusSeverity(status string) string {
	if status == "fail" {
		return "error"
	}
	return "success"
}

// HandleGetDefault returns the connection marked as default (or empty
// when none is set). The chat composer calls this on mount to know
// which connection to pre-select.
func (h *Handler) HandleGetDefault(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	conn, err := h.svc.Default(r.Context())
	if err != nil {
		h.app.Logger.Error("ai providers: get default error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderListFailed, err)
		return
	}
	if conn == nil {
		response.OK(w, nil)
		return
	}
	response.OK(w, conn)
}

// --- OAuth "sign in to connect" ----------------------------------
//
// The three write handlers below drive the RFC 8628 device flow for a
// single connection: begin asks the provider for a user code, poll
// waits for the operator to approve it, disconnect drops the token.
// None of them ever returns the upstream error text -- a token
// endpoint happily echoes the client secret back in an error body.

// HandleOAuthStatus returns the device-flow state for a connection:
// whether it is signed in, which account, and whether a code is
// currently waiting for approval.
func (h *Handler) HandleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	status, err := h.svc.OAuthStatus(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
			return
		}
		h.app.Logger.Error("ai providers: oauth status error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderListFailed, err)
		return
	}
	response.OK(w, status)
}

// HandleOAuthDefaults returns the built-in device-flow endpoint
// prefills, keyed by provider type. Public configuration only: no
// client ID and no secret, because McHarbor registers no OAuth app of
// its own -- the operator brings their own.
func (h *Handler) HandleOAuthDefaults(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	response.OK(w, h.svc.MergedOAuthDefaults())
}

// HandleOAuthBegin starts a device flow and returns the user code plus
// the verification URI the operator has to visit.
func (h *Handler) HandleOAuthBegin(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	status, err := h.svc.OAuthBegin(r.Context(), id)
	if err != nil {
		h.respondOAuthError(w, r, id, "begin", err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.oauth_begin",
		EntityType: "ai_provider_connection",
		EntityID:   id,
		EntityName: h.auditConnectionName(r.Context(), id),
	})
	response.OK(w, status)
}

// HandleOAuthPoll checks whether the operator has approved the pending
// code yet. A still-pending flow is a 200 with pending=true, not an
// error, so the frontend can poll on the provider's own interval.
func (h *Handler) HandleOAuthPoll(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	status, err := h.svc.OAuthPoll(r.Context(), id)
	if err != nil {
		h.respondOAuthError(w, r, id, "poll", err)
		return
	}

	if status.Connected {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "ai_provider.oauth_connected",
			EntityType: "ai_provider_connection",
			EntityID:   id,
			EntityName: status.AccountLabel,
		})
	}
	response.OK(w, status)
}

// HandleOAuthDisconnect drops the stored token, leaving the endpoint
// config in place so the operator can sign in again later.
func (h *Handler) HandleOAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.svc.OAuthDisconnect(r.Context(), id); err != nil {
		h.respondOAuthError(w, r, id, "disconnect", err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "ai_provider.oauth_disconnected",
		EntityType: "ai_provider_connection",
		EntityID:   id,
		EntityName: h.auditConnectionName(r.Context(), id),
	})
	response.OKMsg(w, r, i18n.MsgAIProviderOAuthDisconnected)
}

// respondOAuthError maps the flow's sentinels onto i18n codes. The
// underlying error is logged server-side and never returned: it can
// carry the provider's raw response body.
func (h *Handler) respondOAuthError(w http.ResponseWriter, r *http.Request, id, step string, err error) {
	switch {
	case errors.Is(err, ErrConnectionNotFound):
		response.NotFoundCode(w, r, i18n.ErrAIProviderNotFound)
	case errors.Is(err, ErrOAuthNotConfigured), errors.Is(err, ErrOAuthNoPendingFlow):
		response.BadRequestCode(w, r, i18n.ErrAIProviderOAuthNotConfigured)
	case errors.Is(err, ErrOAuthRejected):
		h.app.Logger.Warn("ai providers: oauth flow rejected", "id", id, "step", step, "error", err)
		response.BadRequestCause(w, r, i18n.ErrAIProviderOAuthRejected, err)
	default:
		h.app.Logger.Error("ai providers: oauth error", "error", err, "id", id, "step", step)
		response.InternalErrorCause(w, r, i18n.ErrAIProviderOAuthFailed, err)
	}
}
