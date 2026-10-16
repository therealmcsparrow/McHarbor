// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deviceauth

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
)

// Handler serves the device flow.
type Handler struct {
	app    *router.AppDeps
	svc    *Service
	grants *apikeys.Service
}

// pollError is the OAuth error string, carried in data.error beside an
// existing i18n code so no new plugin-linked code is needed.
type pollError struct {
	Error string `json:"error"`
}

// HandleStart serves POST /api/device/code.
func (h *Handler) HandleStart(w http.ResponseWriter, r *http.Request) {
	var in StartInput
	if r.ContentLength != 0 {
		if err := response.DecodeBody(r, &in); err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
	}
	res, err := h.svc.Start(r.Context(), in)
	if err != nil {
		h.app.Logger.Error("deviceauth: starting a device request failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.NoStore(w)
	response.OK(w, res)
}

// HandleToken serves POST /api/device/token.
func (h *Handler) HandleToken(w http.ResponseWriter, r *http.Request) {
	var in TokenInput
	if err := response.DecodeBody(r, &in); err != nil || in.DeviceCode == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	key, err := h.svc.Poll(r.Context(), in.DeviceCode)
	switch {
	case errors.Is(err, ErrPending):
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrAuthRequired, pollError{"authorization_pending"})
	case errors.Is(err, ErrSlowDown):
		response.ErrCodeWithData(w, r, http.StatusTooManyRequests, i18n.ErrRateLimitExceed, pollError{"slow_down"})
	case errors.Is(err, ErrExpired):
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrAuthRequired, pollError{"expired_token"})
	case errors.Is(err, ErrDenied):
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, pollError{"access_denied"})
	case errors.Is(err, ErrInvalid):
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, pollError{"invalid_grant"})
	case err != nil:
		h.app.Logger.Error("deviceauth: completing a device request failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	default:
		h.app.AuditLog.Log(r, audit.Entry{
			Action: "create", EntityType: "api_key", EntityID: key.ID, EntityName: key.Name,
			Details: fmt.Sprintf("issued through device sign-in approved by user %s", key.UserID),
		})
		response.NoStore(w)
		response.OK(w, key)
	}
}

// HandleLookup serves GET /api/device/requests/{userCode}.
func (h *Handler) HandleLookup(w http.ResponseWriter, r *http.Request) {
	req, err := h.svc.Lookup(r.Context(), chi.URLParam(r, "userCode"))
	if errors.Is(err, ErrNotFound) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("deviceauth: reading a device request failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, req)
}

// HandleApprove serves POST /api/device/requests/{userCode}/approve.
func (h *Handler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in ApproveInput
	if err := response.DecodeBody(r, &in); err != nil || in.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := h.grants.ValidateGrants(r.Context(), user.ID, in.RoleIDs, in.Scopes, in.EnvironmentIDs, true); err != nil {
		switch {
		case errors.Is(err, apikeys.ErrGrantsRequired):
			response.BadRequestCode(w, r, i18n.ErrAPIKeyGrantsRequired)
		case errors.Is(err, apikeys.ErrGrantInvalid):
			response.BadRequestCode(w, r, i18n.ErrAPIKeyGrantInvalid)
		case errors.Is(err, apikeys.ErrGrantExceedsCaller):
			response.BadRequestCode(w, r, i18n.ErrAPIKeyGrantExceedsCaller)
		default:
			h.app.Logger.Error("deviceauth: grant validation failed", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		}
		return
	}
	code := chi.URLParam(r, "userCode")
	if err := h.svc.Approve(r.Context(), code, user.ID, in); err != nil {
		h.writeDecisionError(w, r, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "approve", EntityType: "device_sign_in", EntityID: NormalizeUserCode(code), EntityName: in.Name,
		Details: fmt.Sprintf("roles=%d scopes=%d environments=%d", len(in.RoleIDs), len(in.Scopes), len(in.EnvironmentIDs)),
	})
	response.OK(w, map[string]string{"status": "approved"})
}

// HandleDeny serves POST /api/device/requests/{userCode}/deny.
func (h *Handler) HandleDeny(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	code := chi.URLParam(r, "userCode")
	if err := h.svc.Deny(r.Context(), code, user.ID); err != nil {
		h.writeDecisionError(w, r, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "deny", EntityType: "device_sign_in", EntityID: NormalizeUserCode(code)})
	response.OK(w, map[string]string{"status": "denied"})
}

func (h *Handler) writeDecisionError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	h.app.Logger.Error("deviceauth: recording a decision failed", "error", err)
	response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
}
