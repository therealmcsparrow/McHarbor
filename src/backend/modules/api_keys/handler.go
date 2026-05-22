// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_keys

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for API key HTTP handlers.
type Handler struct {
	svc  *Service
	rbac *rbac.Service
	app  *router.AppDeps
}

// NewHandler creates a new API keys handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc:  NewService(app.DB),
		rbac: app.RBACService,
		app:  app,
	}
}

// HandleList returns API keys. Admins see all, users see their own.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	// Check if user has admin-level access (wildcard permission)
	isAdmin, _ := h.rbac.HasPermission(user.ID, "", rbac.PermWildcard) // safe: defaults to false (least privilege)

	var userIDFilter string
	if !isAdmin && user.ID != "system" {
		userIDFilter = user.ID
	}

	items, err := h.svc.List(userIDFilter)
	if err != nil {
		h.app.Logger.Error("api_keys: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrAPIKeyListFailed)
		return
	}

	response.OK(w, items)
}

// HandleGet returns a single API key.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	key, err := h.svc.Get(id)
	if err != nil {
		h.app.Logger.Error("api_keys: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if key == nil {
		response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
		return
	}

	response.OK(w, key)
}

// HandleCreate creates a new API key. Returns the plaintext key once.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input CreateAPIKeyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrAPIKeyNameRequired)
		return
	}

	if input.Scopes == nil {
		input.Scopes = []string{}
	}

	result, err := h.svc.Create(user.ID, &input)
	if err != nil {
		h.app.Logger.Error("api_keys: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrAPIKeyCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "api_key",
		EntityID:   result.ID,
		EntityName: input.Name,
	})

	response.Created(w, result)
}

// HandleRevoke deactivates an API key.
func (h *Handler) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	key, err := h.svc.Get(id)
	if err != nil {
		h.app.Logger.Error("api_keys: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if key == nil {
		response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
		return
	}

	if err := h.svc.Revoke(id); err != nil {
		h.app.Logger.Error("api_keys: revoke error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrAPIKeyRevokeFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "revoke",
		EntityType: "api_key",
		EntityID:   id,
		EntityName: key.Name,
	})

	response.OKMsg(w, r, i18n.MsgAPIKeyRevoked)
}
