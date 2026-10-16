// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// TestSessionIDFromCookieIsTheCallersSession guards session management: the
// handlers used to read a context key nothing set, so "sign out other sessions"
// also signed out the caller and the current session was never marked.
func TestSessionIDFromCookieIsTheCallersSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/account/sessions", nil)
	if got := sessionIDFromCookie(req); got != "" {
		t.Fatalf("no cookie: got %q, want empty", got)
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "sess-123"})
	if got, want := sessionIDFromCookie(req), auth.SessionKey("sess-123"); got != want {
		t.Fatalf("with the session cookie: got %q, want the stored key %q", got, want)
	}
}
