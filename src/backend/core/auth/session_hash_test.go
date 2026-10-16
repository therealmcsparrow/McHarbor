// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// TestSessionRowHoldsNoBearerID guards C1: a database or WAL copy must not hand anyone a
// working cookie, so the row id is the hash of the cookie value, never the value itself.
func TestSessionRowHoldsNoBearerID(t *testing.T) {
	svc, database := newSudoTestService(t)
	ctx := context.Background()

	cookie, err := svc.CreateSessionContext(ctx, "user-1")
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}
	var raw, hashed int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, cookie).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, SessionKey(cookie)).Scan(&hashed); err != nil {
		t.Fatal(err)
	}
	if raw != 0 || hashed != 1 {
		t.Fatalf("rows keyed by the raw cookie = %d, by its hash = %d; want 0 and 1", raw, hashed)
	}

	user, err := svc.ValidateSessionContext(ctx, cookie)
	if err != nil || user == nil || user.ID != "user-1" {
		t.Fatalf("validating the cookie: user=%v err=%v", user, err)
	}
	if user, err := svc.ValidateSessionContext(ctx, SessionKey(cookie)); err != nil || user != nil {
		t.Fatalf("the stored key itself must not work as a cookie: user=%v err=%v", user, err)
	}
	if ok, err := svc.TouchSessionContext(ctx, cookie); err != nil || !ok {
		t.Fatalf("touching the session: ok=%v err=%v", ok, err)
	}
	svc.DestroySessionContext(ctx, cookie)
	if user, err := svc.ValidateSessionContext(ctx, cookie); err != nil || user != nil {
		t.Fatalf("a destroyed session still validates: user=%v err=%v", user, err)
	}
}

// TestSessionHashMigrationEndsLegacySessions proves migration 128 removes rows written
// under the raw-id scheme, and the step-up windows bound to them, on an upgraded database.
func TestSessionHashMigrationEndsLegacySessions(t *testing.T) {
	_, database := newSudoTestService(t)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM schema_migrations WHERE name = '128_hash_session_ids.sql'`, nil},
		{`INSERT INTO sessions (id, user_id, expires_at) VALUES ('legacy-raw-cookie', 'user-1', ?)`, []any{future}},
		{`INSERT INTO sudo_windows (id, session_id, user_id, scope, method, expires_at, created_at) VALUES ('w1', 'legacy-raw-cookie', 'user-1', 'secrets.reveal', 'password', ?, ?)`, []any{future, future}},
	} {
		if _, err := database.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seeding %q: %v", q.sql, err)
		}
	}
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("re-running migrations: %v", err)
	}
	for _, table := range []string{"sessions", "sudo_windows"} {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil { // SQL identifier: fixed table names in this test
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s has %d legacy rows after the migration, want 0", table, n)
		}
	}
}
