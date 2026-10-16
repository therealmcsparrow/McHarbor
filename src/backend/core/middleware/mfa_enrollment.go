// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// MFAEnrollmentCheck reports whether user must enrol a second factor before the
// session may reach anything beyond the enrolment routes.
type MFAEnrollmentCheck func(ctx context.Context, user *auth.User) (bool, error)

// mfaEnrollmentExemptPrefixes are what a held session still needs: reading itself,
// signing out, and enrolling. Everything else answers 403 err.mfa.required.
var mfaEnrollmentExemptPrefixes = []string{
	"/api/auth/session",
	"/api/auth/logout",
	"/api/auth/mfa",
}

// MFAEnrollmentExempt reports whether requestPath stays reachable for a session that
// owes MFA enrolment. A non-canonical path is never exempt, so "/api/auth/mfa/../x"
// cannot borrow the enrolment routes' exemption.
func MFAEnrollmentExempt(requestPath string) bool {
	if requestPath == "" || path.Clean(requestPath) != requestPath {
		return false
	}
	for _, prefix := range mfaEnrollmentExemptPrefixes {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}

// MFAEnrollmentGate refuses every request from a session that owes MFA enrolment
// except the exempt routes. It is the enforcement behind the login flag: without it,
// skipping the frontend redirect would leave a fully privileged password-only session.
func MFAEnrollmentGate(owed MFAEnrollmentCheck) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if owed == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil || (r.URL.RawPath == "" && MFAEnrollmentExempt(r.URL.Path)) {
				next.ServeHTTP(w, r)
				return
			}
			required, err := owed(r.Context(), user)
			if err != nil {
				slog.Error("mfa enrollment gate: policy lookup failed", "userId", user.ID, "path", r.URL.Path, "error", err)
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if required {
				response.ForbiddenCode(w, r, i18n.ErrMFARequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
