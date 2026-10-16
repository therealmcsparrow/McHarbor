// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for API key HTTP handlers.
type Handler struct {
	svc *Service
	app *router.AppDeps
}

// NewHandler creates a new API keys handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc: NewService(app.DB, app.RBACService),
		app: app,
	}
}

// HandleList returns API keys. Holders of api_keys.view see all, others their own.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	viewAll, err := h.svc.CanViewAll(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("api_keys: view permission check failed", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	userIDFilter := user.ID
	if viewAll {
		userIDFilter = ""
	}

	items, err := h.svc.List(r.Context(), userIDFilter)
	if err != nil {
		h.app.Logger.Error("api_keys: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyListFailed, err)
		return
	}
	rotateAfter := queryDays(r, "rotateAfterDays")
	expiringWithin := queryDays(r, "expiringWithinDays")
	FlagRotation(items, time.Now().UTC(), rotateAfter, expiringWithin)

	response.OK(w, items)
}

// HandleRotate replaces an active key with a new secret carrying the same
// grants and revokes the old one. The new secret is returned once, here.
func (h *Handler) HandleRotate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	key, ok := h.loadAccessible(w, r, user, id)
	if !ok {
		return
	}
	result, err := h.svc.Rotate(r.Context(), key)
	if errors.Is(err, ErrNotRotatable) {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrAPIKeyRevokeFailed, map[string]string{"reason": "not_rotatable"})
		return
	}
	if err != nil {
		h.app.Logger.Error("api_keys: rotate error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyCreateFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "rotate",
		Severity:   "warning",
		EntityType: "api_key",
		EntityID:   id,
		EntityName: key.Name,
		Details:    "replaced_by=" + result.ID,
	})
	response.Created(w, result)
}

// HandleGrantOptions returns the roles, environments, and permissions the caller may put on a key.
func (h *Handler) HandleGrantOptions(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	options, err := h.svc.GrantOptions(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("api_keys: grant options error", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, options)
}

// HandleCreate creates a new API key for the caller. Returns the plaintext key once.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input CreateAPIKeyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrAPIKeyNameRequired)
		return
	}
	input.RoleIDs = nonNil(input.RoleIDs)
	input.Scopes = nonNil(input.Scopes)
	input.EnvironmentIDs = nonNil(input.EnvironmentIDs)

	if err := h.svc.ValidateGrants(r.Context(), user.ID, input.RoleIDs, input.Scopes, input.EnvironmentIDs, true); err != nil {
		if !h.writeGrantError(w, r, err) {
			h.app.Logger.Error("api_keys: grant validation error", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrAPIKeyCreateFailed, err)
		}
		return
	}

	result, err := h.svc.Create(r.Context(), user.ID, &input)
	if err != nil {
		h.app.Logger.Error("api_keys: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyCreateFailed, err)
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

// HandleUpdate changes name and grants of a key the caller may manage.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	existing, ok := h.loadAccessible(w, r, user, id)
	if !ok {
		return
	}

	var input UpdateAPIKeyInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			response.BadRequestCode(w, r, i18n.ErrAPIKeyNameRequired)
			return
		}
		input.Name = &trimmed
	}

	// Validate against what the key will hold after the patch, so
	// clearing roles while keeping no scopes is rejected.
	roleIDs := input.RoleIDs
	if roleIDs == nil {
		roleIDs = make([]string, 0, len(existing.Roles))
		for _, role := range existing.Roles {
			roleIDs = append(roleIDs, role.ID)
		}
	}
	scopes := input.Scopes
	if scopes == nil {
		scopes = existing.Scopes
	}
	envIDs := input.EnvironmentIDs
	if envIDs == nil {
		envIDs = existing.EnvironmentIDs
	}
	if err := h.svc.ValidateGrants(r.Context(), user.ID, roleIDs, scopes, envIDs, true); err != nil {
		if !h.writeGrantError(w, r, err) {
			h.app.Logger.Error("api_keys: grant validation error", "error", err, "id", id)
			response.InternalErrorCause(w, r, i18n.ErrAPIKeyUpdateFailed, err)
		}
		return
	}

	updated, err := h.svc.Update(r.Context(), id, &input)
	if err != nil {
		h.app.Logger.Error("api_keys: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyUpdateFailed, err)
		return
	}
	if updated == nil {
		response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "api_key",
		EntityID:   id,
		EntityName: updated.Name,
	})

	response.OK(w, updated)
}

// HandleRevoke deactivates an active API key the caller may manage, and
// permanently deletes one that is already revoked. Same route, same
// ownership rule, distinct audit actions.
func (h *Handler) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	key, ok := h.loadAccessible(w, r, user, id)
	if !ok {
		return
	}

	if !key.IsActive {
		if err := h.svc.Delete(r.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
				return
			}
			h.app.Logger.Error("api_keys: delete error", "error", err, "id", id)
			response.InternalErrorCause(w, r, i18n.ErrAPIKeyDeleteFailed, err)
			return
		}

		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "delete",
			EntityType: "api_key",
			EntityID:   id,
			EntityName: key.Name,
		})

		response.OKMsg(w, r, i18n.MsgAPIKeyDeleted)
		return
	}

	if err := h.svc.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
			return
		}
		h.app.Logger.Error("api_keys: revoke error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAPIKeyRevokeFailed, err)
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

// loadAccessible fetches the key and enforces the ownership rule: the
// owner always passes; anyone else needs api_keys.view (the route
// middleware already required api_keys.manage for mutations). It writes
// the error response itself and returns ok=false when the caller must stop.
func (h *Handler) loadAccessible(w http.ResponseWriter, r *http.Request, user *auth.User, id string) (*APIKey, bool) {
	key, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("api_keys: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return nil, false
	}
	if key == nil {
		response.NotFoundCode(w, r, i18n.ErrAPIKeyNotFound)
		return nil, false
	}
	if key.UserID == user.ID {
		return key, true
	}
	viewAll, err := h.svc.CanViewAll(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("api_keys: view permission check failed", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return nil, false
	}
	if !viewAll {
		response.ForbiddenCode(w, r, i18n.ErrAPIKeyForbidden)
		return nil, false
	}
	return key, true
}

// writeGrantError maps the service's sentinel errors to 400 responses.
// It returns false when err is not a grant error.
func (h *Handler) writeGrantError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, ErrGrantsRequired):
		response.BadRequestCode(w, r, i18n.ErrAPIKeyGrantsRequired)
	case errors.Is(err, ErrGrantInvalid):
		response.BadRequestCode(w, r, i18n.ErrAPIKeyGrantInvalid)
	case errors.Is(err, ErrGrantExceedsCaller):
		response.BadRequestCode(w, r, i18n.ErrAPIKeyGrantExceedsCaller)
	default:
		return false
	}
	return true
}

// queryDays reads a non-negative day count from the query string, treating a missing or malformed value as 0 so the flag it feeds stays off.
func queryDays(r *http.Request, name string) int {
	n, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
