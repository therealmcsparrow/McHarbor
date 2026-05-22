// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// validServerTypes defines accepted email server types.
var validServerTypes = map[string]bool{
	"smtp": true, "exchange": true, "gmail": true,
}

// validEncryption defines accepted encryption modes.
var validEncryption = map[string]bool{
	"none": true, "starttls": true, "ssl_tls": true,
}

// validAuthMethods defines accepted SMTP auth methods.
var validAuthMethods = map[string]bool{
	"none": true, "plain": true, "login": true, "cram_md5": true,
}

// Handler holds dependencies for email server HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new email server handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption)
	return &Handler{app: app, service: svc}
}

// HandleList returns all email servers.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("email: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerListFailed)
		return
	}

	response.OK(w, items)
}

// HandleCreate creates a new email server.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateEmailServerInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrEmailServerNameRequired)
		return
	}
	if !validServerTypes[input.ServerType] {
		response.BadRequestCode(w, r, i18n.ErrEmailServerTypeInvalid)
		return
	}
	if input.FromAddress == "" {
		response.BadRequestCode(w, r, i18n.ErrEmailServerFromRequired)
		return
	}

	if code := h.validateByType(input); code != "" {
		response.BadRequestCode(w, r, code)
		return
	}

	srv, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("email: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "email_server.created",
		EntityType: "email_server",
		EntityID:   srv.ID,
		EntityName: srv.Name,
	})

	response.Created(w, srv)
}

// HandleGet returns a single email server.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	srv, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("email: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerListFailed)
		return
	}
	if srv == nil {
		response.NotFoundCode(w, r, i18n.ErrEmailServerNotFound)
		return
	}

	response.OK(w, srv)
}

// HandleUpdate updates an existing email server.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateEmailServerInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	srv, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("email: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerUpdateFailed)
		return
	}
	if srv == nil {
		response.NotFoundCode(w, r, i18n.ErrEmailServerNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "email_server.updated",
		EntityType: "email_server",
		EntityID:   srv.ID,
		EntityName: srv.Name,
	})

	response.OK(w, srv)
}

// HandleDelete removes an email server.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("email: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerRemoveFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrEmailServerNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "email_server.deleted",
		EntityType: "email_server",
		EntityID:   id,
	})

	response.NoContent(w)
}

// HandleSetDefault sets an email server as the default.
func (h *Handler) HandleSetDefault(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.SetDefault(r.Context(), id); err != nil {
		h.app.Logger.Error("email: set default error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "email_server.default_set",
		EntityType: "email_server",
		EntityID:   id,
	})

	response.OKMsg(w, r, i18n.MsgEmailServerDefaultSet)
}

// HandleTest sends a test email via the specified server.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input TestEmailInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.To == "" {
		response.BadRequestCode(w, r, i18n.ErrEmailServerTestToRequired)
		return
	}

	if err := h.service.Test(r.Context(), id, input.To); err != nil {
		h.app.Logger.Error("email: test error", "error", err, "id", id, "to", input.To)
		response.InternalErrorCode(w, r, i18n.ErrEmailServerTestFailed)
		return
	}

	response.OKMsg(w, r, i18n.MsgEmailServerTestSent)
}

// validateByType performs type-specific validation on a create input.
// Returns an empty MsgCode if validation passes.
func (h *Handler) validateByType(input CreateEmailServerInput) i18n.MsgCode {
	switch input.ServerType {
	case "smtp":
		if input.Host == "" {
			return i18n.ErrEmailServerHostRequired
		}
		if input.Port == 0 {
			return i18n.ErrEmailServerPortRequired
		}
		if input.Encryption != "" && !validEncryption[input.Encryption] {
			return i18n.ErrEmailServerEncInvalid
		}
		if input.AuthMethod != "" && !validAuthMethods[input.AuthMethod] {
			return i18n.ErrEmailServerAuthInvalid
		}
		if input.AuthMethod != "none" && input.AuthMethod != "" {
			if input.Username == "" || input.Password == "" {
				return i18n.ErrEmailServerCredRequired
			}
		}

	case "exchange":
		if input.ClientID == "" {
			return i18n.ErrEmailServerClientRequired
		}
		if input.ClientSecret == "" {
			return i18n.ErrEmailServerSecretRequired
		}
		if input.TenantID == "" {
			return i18n.ErrEmailServerTenantRequired
		}

	case "gmail":
		if input.ClientID == "" {
			return i18n.ErrEmailServerClientRequired
		}
		if input.ClientSecret == "" {
			return i18n.ErrEmailServerSecretRequired
		}
	}

	return ""
}
