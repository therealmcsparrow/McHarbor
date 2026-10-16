// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newSyncTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "scim_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("encryption: %v", err)
	}
	return NewService(database, enc), database
}

func seedUser(t *testing.T, database *sql.DB, id, username, email string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO users (id, username, password_hash, email, display_name, is_active) VALUES (?, ?, 'x', ?, ?, 1)`,
		id, username, email, username); err != nil {
		t.Fatalf("seeding user %s: %v", id, err)
	}
}

func fakeDirectory(t *testing.T, users []map[string]any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, err := strconv.Atoi(r.URL.Query().Get("startIndex"))
		if err != nil {
			http.Error(w, "bad startIndex", http.StatusBadRequest)
			return
		}
		count, err := strconv.Atoi(r.URL.Query().Get("count"))
		if err != nil {
			http.Error(w, "bad count", http.StatusBadRequest)
			return
		}
		var resources []map[string]any
		total := 0
		if r.URL.Path == "/Users" {
			total = len(users)
			end := start - 1 + count
			if end > total {
				end = total
			}
			if start-1 < total {
				resources = users[start-1 : end]
			}
		}
		w.Header().Set("Content-Type", "application/scim+json")
		if err := json.NewEncoder(w).Encode(map[string]any{"totalResults": total, "startIndex": start, "Resources": resources}); err != nil {
			t.Errorf("encoding directory page: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSyncPlanApplyAndHistory(t *testing.T) {
	svc, database := newSyncTestService(t)
	ctx := context.Background()
	seedUser(t, database, "u-admin", "root", "")
	seedUser(t, database, "u-alice", "alice", "alice@old.example.com")
	seedUser(t, database, "u-dave", "dave", "")
	if _, err := database.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('sess-dave', 'u-dave', '2999-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seeding session: %v", err)
	}

	users := []map[string]any{
		{"id": "a1", "userName": "alice", "emails": []map[string]any{{"value": "alice@new.example.com", "primary": true}}},
		{"id": "d1", "userName": "dave", "active": false},
	}
	for i := 0; i < 99; i++ {
		users = append(users, map[string]any{"id": fmt.Sprintf("x%03d", i), "userName": fmt.Sprintf("ghost%03d", i)})
	}
	server := fakeDirectory(t, users)

	ep, token, err := svc.CreateEndpoint(ctx, EndpointInput{Name: "idp", BaseURL: server.URL, AuthType: "bearer", Enabled: true}, "u-admin")
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}
	if token == "" {
		t.Fatal("expected a generated token")
	}

	plan, err := svc.PlanSync(ctx, NewPuller(), ep.ID)
	if err != nil {
		t.Fatalf("PlanSync: %v", err)
	}
	if plan.Summary.UsersSeen != 101 {
		t.Fatalf("pagination must read all 101 users, saw %d", plan.Summary.UsersSeen)
	}
	var mappingCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM scim_user_mappings`).Scan(&mappingCount); err != nil {
		t.Fatal(err)
	}
	if mappingCount != 0 {
		t.Fatal("planning must not write mappings")
	}
	if plan.Summary.Linked != 1 || plan.Summary.Disabled != 1 || plan.Summary.Unmatched != 99 {
		t.Fatalf("unexpected summary %+v", plan.Summary)
	}

	if err := svc.ApplyPlan(ctx, ep.ID, plan); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	var daveActive int
	var revoked sql.NullString
	if err := database.QueryRow(`SELECT is_active FROM users WHERE id = 'u-dave'`).Scan(&daveActive); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT revoked_at FROM sessions WHERE id = 'sess-dave'`).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if daveActive != 0 || !revoked.Valid {
		t.Fatalf("dave must be disabled with sessions revoked (active=%d revoked=%v)", daveActive, revoked)
	}

	second, err := svc.PlanSync(ctx, NewPuller(), ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	changes := changesByScimID(*second)
	if got := changes["a1"]; got.Action != ActionUpdate || got.Fields[0] != "email" {
		t.Fatalf("alice is linked now, so her new email is an update: %+v", got)
	}
	if second.Summary.Unmatched != 0 || second.Summary.PendingLink != 99 {
		t.Fatalf("known unmatched rows are pending, not new: %+v", second.Summary)
	}

	if err := svc.RecordRun(ctx, ep.ID, SyncRun{Status: "success", Summary: plan.Summary, Trigger: TriggerManual, Notes: []string{"n1"}}); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if err := svc.RecordRun(ctx, ep.ID, SyncRun{Status: "failed", Error: "boom", DryRun: true, Trigger: TriggerPreview, ActorID: "u-admin"}); err != nil {
		t.Fatalf("RecordRun dry: %v", err)
	}
	history, err := svc.SyncHistory(ctx, ep.ID, 10)
	if err != nil {
		t.Fatalf("SyncHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history rows, got %d", len(history))
	}
	var live, dry SyncLogEntry
	for _, h := range history {
		if h.DryRun {
			dry = h
		} else {
			live = h
		}
	}
	if live.UsersSeen != 101 || live.UsersDisabled != 1 || live.UsersLinked != 1 || live.Trigger != TriggerManual || len(live.Notes) != 1 {
		t.Fatalf("real run row lost counts: %+v", live)
	}
	if dry.Error != "boom" || dry.Trigger != TriggerPreview {
		t.Fatalf("dry run row wrong: %+v", dry)
	}
	after, err := svc.EndpointByID(ctx, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastSyncStatus != "success" || after.UsersCount != 101 {
		t.Fatalf("a dry run must not overwrite the endpoint's last real sync: %+v", after)
	}
}

func TestRotateTokenRevokesTheOldToken(t *testing.T) {
	svc, database := newSyncTestService(t)
	ctx := context.Background()
	seedUser(t, database, "u-admin", "root", "")
	ep, oldToken, err := svc.CreateEndpoint(ctx, EndpointInput{Name: "idp", BaseURL: "http://127.0.0.1:1", AuthType: "bearer", Enabled: true}, "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	newToken, updated, err := svc.RotateToken(ctx, ep.ID)
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if newToken == "" || newToken == oldToken || updated.TokenRotatedAt == "" {
		t.Fatalf("expected a fresh token and a rotation stamp, got %q %+v", newToken, updated)
	}
	if _, err := svc.LookupByBearerToken(ctx, oldToken); err == nil {
		t.Fatal("the old token must stop authenticating")
	}
	if found, err := svc.LookupByBearerToken(ctx, newToken); err != nil || found.ID != ep.ID {
		t.Fatalf("the new token must authenticate, got %v %v", found, err)
	}

	basic, _, err := svc.CreateEndpoint(ctx, EndpointInput{Name: "basic", BaseURL: "http://127.0.0.1:1", AuthType: "basic", Username: "u", BearerToken: "pw", Enabled: true}, "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RotateToken(ctx, basic.ID); !errors.Is(err, ErrTokenRotationUnsupported) {
		t.Fatalf("basic-auth endpoints have no issued token to rotate, got %v", err)
	}
}
