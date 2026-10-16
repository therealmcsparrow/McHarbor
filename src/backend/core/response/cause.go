// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package response

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/redact"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
)

// ErrCodeCause sends a coded failure that also carries err's redacted text as
// detail. The code says what failed; the detail says why, in the words of the
// daemon, the agent or the parser that refused — the part an operator can act on.
// The caller still logs err itself; redaction makes the detail safe, not complete.
func ErrCodeCause(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode, err error) {
	ErrCodeCauseWithData(w, r, status, code, err, nil)
}

// ErrCodeCauseWithData is ErrCodeCause for a failure that also returns structure.
func ErrCodeCauseWithData(w http.ResponseWriter, r *http.Request, status int, code i18n.MsgCode, err error, data any) {
	msg := i18n.T(i18n.FromContext(r.Context()), code)
	writeJSON(w, status, APIResponse{
		Success:   false,
		Error:     msg,
		Code:      string(code),
		Detail:    detailFor(r.Context(), err, msg),
		Data:      data,
		RequestID: requestid.From(r.Context()),
	})
}

type detailKey struct{}

// WithDetail marks ctx as an authenticated request whose failures may carry
// their cause. Pre-auth routes never get it, so a login or setup failure cannot
// tell a signed-out caller anything the code does not.
func WithDetail(ctx context.Context) context.Context {
	return context.WithValue(ctx, detailKey{}, true)
}

// DetailFor is the cause a plugin's own envelope may carry for r: err's redacted
// text on an authenticated request, "" on a pre-auth one. Plugins write their
// own envelopes, so this is their one way to apply the host's gate and redactor.
func DetailFor(r *http.Request, err error, summary string) string {
	return detailFor(r.Context(), err, summary)
}

func detailFor(ctx context.Context, err error, summary string) string {
	if allowed, _ := ctx.Value(detailKey{}).(bool); !allowed {
		return ""
	}
	return Detail(err, summary)
}

// Detail returns err's redacted text, or "" when it adds nothing to summary —
// including sql.ErrNoRows, whose text only restates a not-found code.
func Detail(err error, summary string) string {
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	d := redact.ErrorDetail(err.Error())
	if d == summary {
		return ""
	}
	return d
}

// BadRequestCause sends 400 with a translated code and err's redacted detail.
func BadRequestCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusBadRequest, code, err)
}

// UnauthorizedCause sends 401 with a translated code and err's redacted detail.
func UnauthorizedCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusUnauthorized, code, err)
}

// ForbiddenCause sends 403 with a translated code and err's redacted detail.
func ForbiddenCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusForbidden, code, err)
}

// NotFoundCause sends 404 with a translated code and err's redacted detail.
func NotFoundCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusNotFound, code, err)
}

// ConflictCause sends 409 with a translated code and err's redacted detail.
func ConflictCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusConflict, code, err)
}

// InternalErrorCause sends 500 with a translated code and err's redacted detail.
func InternalErrorCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusInternalServerError, code, err)
}

// ServiceUnavailableCause sends 503 with a translated code and err's redacted
// detail — the upstream's own refusal is exactly what this status is about.
func ServiceUnavailableCause(w http.ResponseWriter, r *http.Request, code i18n.MsgCode, err error) {
	ErrCodeCause(w, r, http.StatusServiceUnavailable, code, err)
}
