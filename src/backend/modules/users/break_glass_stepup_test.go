// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestBreakGlassNeedsAStepUpWindow: activation grants everything, so a hijacked
// session of a designated account is refused until the person re-authenticates.
func TestBreakGlassNeedsAStepUpWindow(t *testing.T) {
	database, svc := openGovernanceDB(t)
	ctx := context.Background()
	if err := svc.Designate(ctx, "u1", "a1", DesignateInput{MaxMinutes: 60}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, 'u1', ?)`, auth.SessionKey("sess-u1"),
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	h := &Handler{service: svc, app: &router.AppDeps{
		AuthService: auth.NewService(database),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/break-glass/activate", strings.NewReader(`{"reason":"datacenter on fire","minutes":30}`))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "sess-u1"})
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "user1"}))
	rec := httptest.NewRecorder()
	h.HandleBreakGlassActivate(rec, req)

	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "sudo") {
		t.Fatalf("activation without a step-up window: %d %s", rec.Code, rec.Body.String())
	}
	if st, err := svc.BreakGlassStatusFor(ctx, "u1"); err != nil || st.Active != nil {
		t.Fatalf("a refused activation must not open a session: %+v %v", st, err)
	}
}

// TestOneOpenBreakGlassSessionPerUser: the partial unique index is what stops two
// concurrent activations both opening a session.
func TestOneOpenBreakGlassSessionPerUser(t *testing.T) {
	database, _ := openGovernanceDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	later := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	insert := func(id string) error {
		_, err := database.Exec(`INSERT INTO break_glass_sessions (id, user_id, reason, started_at, expires_at) VALUES (?, 'u1', 'datacenter on fire', ?, ?)`, id, now, later)
		return err
	}
	if err := insert("bg-1"); err != nil {
		t.Fatal(err)
	}
	if err := insert("bg-2"); err == nil {
		t.Fatal("a second open break-glass session for the same user must be refused by the database")
	}
}
