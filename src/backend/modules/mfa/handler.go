// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package mfa

import (
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// issuer is the label authenticator apps show above the code. It is
// intentionally the product name rather than the hostname so a user
// with several McHarbor instances still recognises the entry, and so
// the QR contents never leak an internal hostname.
const issuer = "McHarbor"

// Handler serves the per-user MFA endpoints.
type Handler struct {
	app      *router.AppDeps
	svc      *Service
	enforcer *Enforcer
}

// NewHandler creates an MFA handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption)
	return &Handler{app: app, svc: svc, enforcer: NewEnforcer(svc, rbac.NewMFAPolicy(app.DB, app.RBACService))}
}

// HandleStatus returns the caller's own MFA state.
// GET /auth/mfa/status
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	status, err := h.svc.Status(r.Context(), user.ID)
	if err != nil {
		h.app.Logger.Error("mfa status failed", "userId", user.ID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrMFAStatusFailed, err)
		return
	}

	response.OK(w, status)
}

// HandleBeginEnrollment issues a new secret and recovery codes. The
// response is the only time either is ever readable.
// POST /auth/mfa/enroll/begin
func (h *Handler) HandleBeginEnrollment(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	enrollment, err := h.svc.BeginEnrollment(r.Context(), user.ID, user.Username, issuer)
	if err != nil {
		if errors.Is(err, ErrAlreadyEnabled) {
			response.ConflictCode(w, r, i18n.ErrMFAAlreadyEnabled)
			return
		}
		h.app.Logger.Error("mfa enrollment start failed", "userId", user.ID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrMFAEnrollFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "mfa_enroll_begin",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	response.OK(w, enrollment)
}

// HandleConfirmEnrollment verifies the first code and turns MFA on.
// POST /auth/mfa/enroll/confirm
func (h *Handler) HandleConfirmEnrollment(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req confirmRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Code == "" {
		response.BadRequestCode(w, r, i18n.ErrMFAInvalidCode)
		return
	}

	if err := h.svc.ConfirmEnrollment(r.Context(), user.ID, req.Code); err != nil {
		switch {
		case errors.Is(err, ErrInvalidCode):
			response.BadRequestCode(w, r, i18n.ErrMFAInvalidCode)
		case errors.Is(err, ErrNotEnrolled):
			response.BadRequestCode(w, r, i18n.ErrMFANotEnrolled)
		case errors.Is(err, ErrAlreadyEnabled):
			response.ConflictCode(w, r, i18n.ErrMFAAlreadyEnabled)
		default:
			h.app.Logger.Error("mfa enrollment confirm failed", "userId", user.ID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrMFAEnrollFailed, err)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "mfa_enabled",
		Severity:   "warning",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	response.OK(w, map[string]bool{"ok": true})
}

// HandleDisable turns MFA off for the caller and destroys the secret.
// DELETE /auth/mfa
func (h *Handler) HandleDisable(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	// A covered user switching their factor off would only be held at the enrolment
	// screen on the next request; refusing here keeps the policy's intent visible.
	required, err := h.enforcer.Required(r.Context(), user)
	if err != nil {
		h.app.Logger.Error("mfa policy lookup failed", "userId", user.ID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrMFADisableFailed, err)
		return
	}
	if required {
		response.ForbiddenCode(w, r, i18n.ErrMFARequired)
		return
	}

	if err := h.svc.Disable(r.Context(), user.ID); err != nil {
		if errors.Is(err, ErrNotEnrolled) {
			response.BadRequestCode(w, r, i18n.ErrMFANotEnrolled)
			return
		}
		h.app.Logger.Error("mfa disable failed", "userId", user.ID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrMFADisableFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "mfa_disabled",
		Severity:   "warning",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	response.OK(w, map[string]bool{"disabled": true})
}
