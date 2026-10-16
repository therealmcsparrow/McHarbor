// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// TestSessionManagementWorksOnHashedIDs walks the sessions screen end to end after ids are
// stored hashed: the listed id is not the cookie, the caller's row is still marked current,
// revoking by the listed id ends the other session, and "revoke all" keeps the caller's.
func TestSessionManagementWorksOnHashedIDs(t *testing.T) {
	database, svc := openGovernanceDB(t)
	authSvc := auth.NewService(database)
	ctx := context.Background()

	mine, err := authSvc.CreateSessionContext(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := authSvc.CreateSessionContext(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	third, err := authSvc.CreateSessionContext(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: mine})
	current := sessionIDFromCookie(req)

	items, err := svc.ListSessions(ctx, "u1", current)
	if err != nil {
		t.Fatal(err)
	}
	var currentRows int
	otherID := ""
	for _, it := range items {
		if it.ID == mine || it.ID == other || it.ID == third {
			t.Fatalf("the sessions list returned a bearer cookie value as a row id: %q", it.ID)
		}
		if it.CurrentSession {
			currentRows++
		}
		if it.ID == auth.SessionKey(other) {
			otherID = it.ID
		}
	}
	if len(items) != 3 || currentRows != 1 || otherID == "" {
		t.Fatalf("list: %d rows, %d marked current, other found=%v; want 3, 1, true", len(items), currentRows, otherID != "")
	}

	if err := svc.RevokeSession(ctx, "u1", otherID, "u1"); err != nil {
		t.Fatalf("revoking by the listed id: %v", err)
	}
	if u, err := authSvc.ValidateSessionContext(ctx, other); err != nil || u != nil {
		t.Fatalf("a revoked session still validates: user=%v err=%v", u, err)
	}

	if _, err := svc.RevokeAllSessions(ctx, "u1", current, "u1"); err != nil {
		t.Fatal(err)
	}
	if u, err := authSvc.ValidateSessionContext(ctx, third); err != nil || u != nil {
		t.Fatalf("revoke-all left another session alive: user=%v err=%v", u, err)
	}
	if u, err := authSvc.ValidateSessionContext(ctx, mine); err != nil || u == nil {
		t.Fatalf("revoke-all ended the caller's own session: user=%v err=%v", u, err)
	}
}
