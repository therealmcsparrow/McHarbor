// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"errors"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// Phase 4 password-reset flow.
//
// Two-step:
//   1. Request a reset link. Public, no auth.
//        POST /auth/password/forgot  {identifier|email|username}
//   2. Consume the link and set a new password.
//        POST /auth/password/reset   {token, newPassword}
//
// For email enumeration prevention: the /forgot endpoint always
// returns 200 with a neutral "if an account exists we sent a link"
// message, regardless of whether the username/email matched. The
// only signal to the caller is the i18n code, which is constant.
//
// Tokens are single-use, hashed-at-rest, TTL-bound, and contain
// 256 bits of entropy (see IssuePasswordResetToken in account.go).

// ForgotPasswordRequest is the body for POST /auth/password/forgot.
type ForgotPasswordRequest struct {
	// Identifier is the single field the reset form actually has: one
	// box the account holder types either their username or their
	// email address into. The server does not care which it is — every
	// non-empty value below is matched against both columns.
	//
	// Email and Username remain for callers that know which of the two
	// they hold. Any combination may be sent; blank fields are ignored
	// rather than matched, so an omitted field can never select a row
	// whose column happens to be empty.
	Identifier string `json:"identifier"`
	Email      string `json:"email"`
	Username   string `json:"username"`
}

// ForgotPasswordResponse is the always-200 response. The caller
// cannot tell from this whether the account exists.
type ForgotPasswordResponse struct {
	Message string `json:"message"`
}

// ResetPasswordRequest is the body for POST /auth/password/reset.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// ResetPasswordResponse is the success response.
type ResetPasswordResponse struct {
	Message string `json:"message"`
}

// HandleForgotPassword is public (no auth) and constant-time from
// the caller's perspective. It looks up the user, issues a reset
// token, and (best-effort) emails it. Errors are swallowed into
// the neutral response.
func (h *Handler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := response.DecodeBody(r, &req); err != nil {
		// Still respond 200 with neutral message — do not leak
		// that the body was malformed.
		response.OK(w, ForgotPasswordResponse{
			Message: "If an account exists, a reset link has been sent.",
		})
		return
	}
	identifiers := resetIdentifiers(req.Identifier, req.Email, req.Username)
	if len(identifiers) == 0 {
		response.OK(w, ForgotPasswordResponse{
			Message: "If an account exists, a reset link has been sent.",
		})
		return
	}

	// Look up the user. Every identifier is tried against both the
	// username and the email column, so the form can keep offering one
	// box and the account holder never has to know which of the two
	// McHarbor stored. The query is deliberately broad for the same
	// reason it answers uniformly: a caller must not be able to
	// enumerate by which field was accepted.
	var (
		userID      string
		username    string
		displayName *string
		email       *string
		managed     bool
	)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(identifiers)), ",")
	args := make([]any, 0, len(identifiers)*2)
	for range 2 {
		for _, identifier := range identifiers {
			args = append(args, identifier)
		}
	}
	//nolint:gosec // G202: the concatenated part is a list of "?" markers, every value is bound
	query := `
		SELECT id, username, display_name, email, (auth_provider != 'local')
		  FROM users
		 WHERE (LOWER(username) IN (` + placeholders + `) OR LOWER(email) IN (` + placeholders + `))
		   AND id != 'system' AND id != '__legacy_owner__'
		 LIMIT 1
	`
	row := h.app.DB.QueryRowContext(r.Context(), query, args...)
	if err := row.Scan(&userID, &username, &displayName, &email, &managed); err != nil {
		// Unknown user OR not local — same response.
		response.OK(w, ForgotPasswordResponse{
			Message: "If an account exists, a reset link has been sent.",
		})
		return
	}
	if managed {
		// IdP-managed users must reset through the IdP.
		response.OK(w, ForgotPasswordResponse{
			Message: "If an account exists, a reset link has been sent.",
		})
		return
	}

	// Delivery settings decide the link's lifetime, so they are read
	// before the token is minted rather than after. A failure here
	// falls back to the compiled-in TTL — the reset still works, it
	// just ignores an unreadable settings row.
	emailSettings, err := h.service.ResetEmailSettings(r.Context())
	if err != nil {
		h.app.Logger.Error("users: reading password reset email settings", "error", err)
		emailSettings = ResetEmailSettings{TTL: PasswordResetEmailTTL}
	}

	// Issue the token. Email delivery is best-effort; we never
	// surface a failure to the caller (would enable enumeration).
	plaintext, expiresAt, err := h.service.IssuePasswordResetToken(r.Context(),
		userID, "self", emailSettings.TTL,
	)
	if err != nil {
		h.app.Logger.Error("users: issue reset token failed", "error", err, "userId", userID)
		response.OK(w, ForgotPasswordResponse{
			Message: "If an account exists, a reset link has been sent.",
		})
		return
	}

	// Best-effort email send. Delivery failures are logged but
	// do NOT change the response (enumeration prevention).
	switch {
	case email == nil || *email == "":
		// No email on file — log so admins can help manually.
		h.app.Logger.Info("users: password reset requested but user has no email; manual delivery required",
			"userId", userID)
	case !emailSettings.Configured():
		// No channel picked in Security -> Password. Same shape of
		// problem as the line above: the token is valid, nothing can
		// carry it, and only an admin can fix that.
		h.app.Logger.Warn("users: password reset requested but no email channel is configured; "+
			"pick one in Security > Password", "userId", userID)
	default:
		recipient := resetRecipient{Username: username, Email: *email}
		if displayName != nil {
			recipient.DisplayName = *displayName
		}
		if sendErr := h.sendResetEmail(r.Context(), r, recipient, plaintext, expiresAt, emailSettings); sendErr != nil {
			h.app.Logger.Error("users: password reset email delivery failed",
				"error", sendErr, "userId", userID, "channelId", emailSettings.ChannelID)
		}
	}

	response.OK(w, ForgotPasswordResponse{
		Message: "If an account exists, a reset link has been sent.",
	})
}

// resetRecipient is the account half of the reset mail's template
// variables. Carried as a struct so sendResetEmail's signature does not
// grow a run of interchangeable strings.
type resetRecipient struct {
	Username    string
	DisplayName string
	Email       string
}

// resetIdentifiers normalises the account identifiers a /forgot body
// carried into the set the lookup may match on.
//
// Blank values are dropped rather than passed through: a request that
// omits `email` must not be able to match a row whose email column is
// the empty string, which is what the previous
// `LOWER(email) = LOWER(”)` comparison allowed. Values are lowercased
// here so the SQL can compare against LOWER(column) with plain
// equality, and de-duplicated so a client that sends the same string
// as both `identifier` and `username` does not widen the IN list.
func resetIdentifiers(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, duplicate := seen[normalized]; duplicate {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

// HandleResetPassword is public and consumes a token. The token
// must be valid, unexpired, and unconsumed.
func (h *Handler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Token == "" {
		response.BadRequestCode(w, r, i18n.ErrPasswordResetInvalid)
		return
	}
	if req.NewPassword == "" {
		response.BadRequestCode(w, r, i18n.ErrUserPasswordRequired)
		return
	}

	ip := httpx.ClientIP(r)
	ua := r.Header.Get("User-Agent")

	// Resolve (but do not yet consume) the token so a password that
	// the policy rejects does not burn the user's single-use link.
	_, userID, err := h.service.ResolvePasswordResetToken(r.Context(), req.Token)
	switch {
	case errors.Is(err, ErrPasswordResetTokenInvalid):
		response.BadRequestCode(w, r, i18n.ErrPasswordResetInvalid)
		return
	case errors.Is(err, ErrPasswordResetTokenExpired):
		response.BadRequestCode(w, r, i18n.ErrPasswordResetExpired)
		return
	case errors.Is(err, ErrPasswordResetTokenConsumed):
		response.BadRequestCode(w, r, i18n.ErrPasswordResetConsumed)
		return
	case err != nil:
		h.app.Logger.Error("users: resolve reset token failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrPasswordResetFailed, err)
		return
	}

	// Policy gate before the token is spent.
	policy := auth.LoadPasswordPolicy(r.Context(), h.app.DB)
	if verr := auth.ValidatePassword(req.NewPassword, policy); verr != nil {
		var pverr *auth.PasswordValidationError
		if errors.As(verr, &pverr) {
			respondPasswordPolicyError(w, r, pverr)
			return
		}
		response.BadRequestCause(w, r, i18n.ErrPasswordTooWeak, verr)
		return
	}

	if _, err := h.service.ConsumePasswordResetToken(r.Context(), req.Token, ip, ua); err != nil {
		h.app.Logger.Error("users: consume reset token failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrPasswordResetFailed, err)
		return
	}

	if err := h.service.ChangePassword(r.Context(), userID, req.NewPassword); err != nil {
		var pverr *auth.PasswordValidationError
		if errors.As(err, &pverr) {
			respondPasswordPolicyError(w, r, pverr)
			return
		}
		h.app.Logger.Error("users: change password after reset failed", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserUpdateFailed, err)
		return
	}

	// Forcibly revoke all sessions so a stolen token can't leave
	// the attacker logged in. The user will re-authenticate.
	if _, err := h.service.RevokeAllSessions(r.Context(), userID, "", "password-reset"); err != nil {
		h.app.Logger.Warn("users: revoke all after password reset failed", "error", err, "userId", userID)
	}

	response.OK(w, ResetPasswordResponse{Message: "password has been reset"})
}
