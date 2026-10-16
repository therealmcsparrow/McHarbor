// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestSudoAttemptsAreThrottledPerUserAndAddress guards the step-up password
// check: a hijacked session gets sudoAttemptsPerMinute guesses, not unlimited
// ones, and another user on the same address keeps their own allowance.
func TestSudoAttemptsAreThrottledPerUserAndAddress(t *testing.T) {
	h := &Handler{app: &router.AppDeps{
		ResolveClientAddress: func(*http.Request) (string, string, string) { return "", "203.0.113.7", "test" },
	}}
	guarded := middleware.RateLimit(sudoAttemptsPerMinute, h.sudoAttemptKey)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	attempt := func(userID string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/sudo", nil)
		req = req.WithContext(context.WithValue(req.Context(), coreauth.UserContextKey, &coreauth.User{ID: userID}))
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < sudoAttemptsPerMinute; i++ {
		if got := attempt("victim"); got != http.StatusOK {
			t.Fatalf("attempt %d within the allowance: got %d", i+1, got)
		}
	}
	if got := attempt("victim"); got != http.StatusTooManyRequests {
		t.Fatalf("the attempt past the allowance: got %d, want 429", got)
	}
	if got := attempt("colleague"); got != http.StatusOK {
		t.Fatalf("another user on the same address must keep their own allowance: got %d", got)
	}
}
