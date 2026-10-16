// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	instancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// SudoRequest is the body of POST /auth/sudo. Exactly one of the two proofs
// is used; a TOTP code is preferred when the account has a second factor,
// because it proves possession rather than knowledge.
type SudoRequest struct {
	Scope    string `json:"scope"`
	Password string `json:"password"`
	TOTPCode string `json:"totpCode"`
}

// SudoResponse tells the client how long the window lasts so it can show a
// countdown rather than discovering the expiry through a 403.
type SudoResponse struct {
	ExpiresAt string `json:"expiresAt"`
	ExpiresIn int    `json:"expiresIn"`
}

// allowedSudoScopes is the closed set a client may ask for. An open string
// would let a caller mint a window named whatever the *next* sensitive action
// happens to check for, before that action exists.
var allowedSudoScopes = map[string]bool{
	coreauth.SudoScopeSecretsReveal:     true,
	coreauth.SudoScopeSessionTerminate:  true,
	coreauth.SudoScopeEnvironmentDelete: true,
	coreauth.SudoScopeBreakGlass:        true,
	instancebackup.SudoScopeDownload:    true,
}

// HandleSudo re-verifies the caller and opens a five-minute step-up window
// bound to their session.
//
// Self-service and deliberately ungated by RBAC: it acts on the caller's own
// session, proves nothing they did not already prove at login, and grants
// nothing on its own — a window is only ever *additional* to the permission
// the action already requires. The reveal endpoint needs both.
//
// POST /api/auth/sudo
func (h *Handler) HandleSudo(w http.ResponseWriter, r *http.Request) {
	user := coreauth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req SudoRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if !allowedSudoScopes[scope] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// The session id is the cookie value; there is no parameter to spoof.
	cookie, err := r.Cookie(coreauth.SessionCookie)
	if err != nil || cookie.Value == "" {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var verifier coreauth.TOTPVerifier
	if h.mfa != nil {
		verifier = h.mfa
	}

	expiresAt, err := h.app.AuthService.MintSudoWindow(
		r.Context(), user.ID, cookie.Value, scope, req.Password, req.TOTPCode, verifier,
	)
	if err != nil {
		if errors.Is(err, coreauth.ErrSudoVerificationFailed) {
			// Audited: a failed step-up in front of a credential disclosure is
			// exactly the signal worth keeping.
			h.app.AuditLog.Log(r, audit.Entry{
				Action:     "sudo_failed",
				Severity:   "warning",
				EntityType: "session",
				EntityName: scope,
			})
			response.ForbiddenCode(w, r, i18n.ErrSudoFailed)
			return
		}
		h.app.Logger.Error("auth: step-up failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "sudo_granted",
		EntityType: "session",
		EntityName: scope,
	})

	response.NoStore(w)
	response.OK(w, SudoResponse{
		ExpiresAt: expiresAt.Format(time.RFC3339),
		ExpiresIn: int(time.Until(expiresAt).Seconds()),
	})
}

// sudoAttemptsPerMinute bounds password guesses through a step-up. The window
// unlocks secret reveals, so a hijacked session must not get to brute-force the
// account password here at the pace of the network.
const sudoAttemptsPerMinute = 5

// sudoAttemptKey buckets step-up attempts per user and per client address, so
// one attacker cannot spread guesses over accounts or one account over addresses
// without each bucket running dry on its own.
func (h *Handler) sudoAttemptKey(r *http.Request) string {
	user := coreauth.UserFromContext(r.Context())
	if user == nil {
		return ""
	}
	return user.ID + "|" + h.app.ClientKey(r)
}
