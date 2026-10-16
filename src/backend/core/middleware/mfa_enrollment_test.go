// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

func serveMFAGate(t *testing.T, owed MFAEnrollmentCheck, user *auth.User, target string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	handler := MFAEnrollmentGate(owed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if user != nil {
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, user))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, reached
}

func owesEnrollment(context.Context, *auth.User) (bool, error) { return true, nil }

func TestMFAEnrollmentGateBlocksOwingSession(t *testing.T) {
	rec, reached := serveMFAGate(t, owesEnrollment, &auth.User{ID: "u1"}, "/api/containers")
	if reached {
		t.Fatal("a session owing enrolment must not reach a normal route")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Success || body.Code != string(i18n.ErrMFARequired) {
		t.Fatalf("body = %+v, want code %s", body, i18n.ErrMFARequired)
	}
}

func TestMFAEnrollmentGateAllowsEnrollmentRoutes(t *testing.T) {
	for _, target := range []string{
		"/api/auth/session",
		"/api/auth/session/policy",
		"/api/auth/logout",
		"/api/auth/mfa",
		"/api/auth/mfa/status",
		"/api/auth/mfa/enroll/begin",
		"/api/auth/mfa/enroll/confirm",
	} {
		rec, reached := serveMFAGate(t, owesEnrollment, &auth.User{ID: "u1"}, target)
		if !reached || rec.Code != http.StatusNoContent {
			t.Fatalf("%s: reached=%v status=%d, want the handler to run", target, reached, rec.Code)
		}
	}
}

func TestMFAEnrollmentGateRefusesLookalikePaths(t *testing.T) {
	for _, target := range []string{
		"/api/auth/mfa/../../containers",
		"/api/auth/mfaX",
		"/api/auth/sessions",
		"/api/auth/me/permissions",
	} {
		_, reached := serveMFAGate(t, owesEnrollment, &auth.User{ID: "u1"}, target)
		if reached {
			t.Fatalf("%s must not borrow the enrolment exemption", target)
		}
	}
}

func TestMFAEnrollmentGatePassesWhenNotOwed(t *testing.T) {
	notOwed := func(context.Context, *auth.User) (bool, error) { return false, nil }
	if _, reached := serveMFAGate(t, notOwed, &auth.User{ID: "u1"}, "/api/containers"); !reached {
		t.Fatal("a user who owes nothing must pass")
	}
	if _, reached := serveMFAGate(t, nil, &auth.User{ID: "u1"}, "/api/containers"); !reached {
		t.Fatal("a nil check gates nothing")
	}
	if _, reached := serveMFAGate(t, owesEnrollment, nil, "/api/containers"); !reached {
		t.Fatal("with no user in context the gate defers to the auth middleware")
	}
}

func TestMFAEnrollmentGateFailsClosedOnLookupError(t *testing.T) {
	broken := func(context.Context, *auth.User) (bool, error) { return false, errors.New("db down") }
	rec, reached := serveMFAGate(t, broken, &auth.User{ID: "u1"}, "/api/containers")
	if reached || rec.Code != http.StatusInternalServerError {
		t.Fatalf("reached=%v status=%d, want a closed 500", reached, rec.Code)
	}
}
