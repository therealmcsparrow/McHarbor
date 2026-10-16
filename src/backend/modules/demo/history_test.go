// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	modaudit "github.com/therealmcsparrow/mcharbor/modules/audit"
)

const historyTestSecret = "history-test-secret"

func historyDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "demo.db")+"?_foreign_keys=OFF")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := db.Migrate(conn, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return conn
}

// seedRealAccount stands in for what the builder copies from the running
// database: one real, login-capable admin and one audit row this boot wrote.
func seedRealAccount(t *testing.T, conn *sql.DB, now time.Time) {
	t.Helper()
	ts := now.UTC().Format(time.RFC3339)
	if _, err := conn.Exec(`INSERT INTO users (id, username, email, password_hash, display_name, role, created_at, updated_at)
		VALUES ('real-admin', 'carlo', 'carlo@example.com', '$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA', 'Carlo', 'admin', ?, ?)`, ts, ts); err != nil {
		t.Fatal(err)
	}
	row := audit.HashRow{ID: "boot-row", Action: "login", Severity: "success", UserID: sp("real-admin"), Username: sp("carlo"),
		Timestamp: ts, CreatedAt: ts, UpdatedAt: ts}
	hash, err := audit.ComputeRowHash(historyTestSecret, row)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO audit_logs (id, user_id, username, action, severity, timestamp, created_at, updated_at, row_hash)
		VALUES ('boot-row', 'real-admin', 'carlo', 'login', 'success', ?, ?, ?, ?)`, ts, ts, ts, hash); err != nil {
		t.Fatal(err)
	}
}

func historyEstate(now time.Time) (*estate.Estate, map[string]string) {
	e := estate.Build(estate.DefaultSeed, now)
	ids := make(map[string]string, len(e.Hosts))
	for _, h := range e.Hosts {
		ids[h.Key] = stableID("env", h.Key)
	}
	return e, ids
}

func count(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func seededHistory(t *testing.T) (*sql.DB, map[string]string, time.Time) {
	t.Helper()
	conn := historyDB(t)
	now := time.Now()
	seedRealAccount(t, conn, now)
	e, envIDs := historyEstate(now)
	if err := seedHistory(context.Background(), conn, e, envIDs, now, historyTestSecret); err != nil {
		t.Fatal(err)
	}
	return conn, envIDs, now
}

func TestHistorySeedsEveryKind(t *testing.T) {
	conn, _, _ := seededHistory(t)
	cases := []struct {
		name     string
		query    string
		min, max int
	}{
		{"audit rows", "SELECT COUNT(*) FROM audit_logs", 20000, 20001},
		{"failed audit rows", "SELECT COUNT(*) FROM audit_logs WHERE severity = 'error'", 200, 2000},
		{"workflows", "SELECT COUNT(*) FROM workflows", 38, 42},
		{"workflow runs", "SELECT COUNT(*) FROM workflow_runs", 40, 300},
		{"failed workflow runs", "SELECT COUNT(*) FROM workflow_runs WHERE status = 'failed'", 1, 100},
		{"schedules", "SELECT COUNT(*) FROM schedules", 55, 60},
		{"schedule failures", "SELECT COUNT(*) FROM schedule_executions WHERE status = 'failure'", 1, 200},
		{"alert rules", "SELECT COUNT(*) FROM alerts", 20, 20},
		{"alert events", "SELECT COUNT(*) FROM alert_events", 100, 1000},
		{"incidents", "SELECT COUNT(*) FROM incidents", 15, 15},
		{"open incidents", "SELECT COUNT(*) FROM incidents WHERE status = 'open'", 5, 5},
		{"resolved with timeline", "SELECT COUNT(*) FROM incidents WHERE status = 'resolved' AND timeline != '' AND resolved_at IS NOT NULL", 10, 10},
		{"demo users", "SELECT COUNT(*) FROM users WHERE email LIKE '%@acme.example'", 25, 25},
		{"demo groups", "SELECT COUNT(*) FROM groups WHERE name IN ('Platform', 'SRE', 'Developers', 'Store Ops')", 4, 4},
		{"group members", "SELECT COUNT(*) FROM group_members", 24, 24},
		{"group roles", "SELECT COUNT(*) FROM group_roles WHERE group_id IN (SELECT id FROM groups WHERE is_system = 0)", 4, 4},
		{"completion marker", "SELECT COUNT(*) FROM settings WHERE key = 'demo.history_seeded' AND value = '1'", 1, 1},
	}
	for _, c := range cases {
		if n := count(t, conn, c.query); n < c.min || n > c.max {
			t.Errorf("%s: got %d, want %d..%d", c.name, n, c.min, c.max)
		}
	}
}

func TestHistoryNothingSeededCanActOnItsOwn(t *testing.T) {
	conn, envIDs, _ := seededHistory(t)
	if n := count(t, conn, "SELECT COUNT(*) FROM workflows WHERE status = 'active'"); n != 0 {
		t.Errorf("%d seeded workflows are active and would be armed by the trigger service", n)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM alert_destinations"); n != 0 {
		t.Errorf("%d alert destinations seeded; an alert must never be delivered anywhere", n)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM alerts WHERE send_in_app != 0 OR COALESCE(channel_id, '') != ''"); n != 0 {
		t.Errorf("%d alerts would notify", n)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM alerts WHERE condition_key = 'image_update'"); n != 0 {
		t.Errorf("%d image-update alerts would query a registry", n)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM schedules WHERE enabled = 1 AND action NOT IN ('start', 'stop', 'restart')"); n != 0 {
		t.Errorf("%d enabled schedules do more than a container lifecycle call", n)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM schedules WHERE enabled = 1"); n == 0 {
		t.Error("no enabled schedules; the lifecycle ones are meant to show as active")
	}
	rows, err := conn.Query("SELECT DISTINCT env_id FROM schedules")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()
	known := make(map[string]bool, len(envIDs))
	for _, id := range envIDs {
		known[id] = true
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if !known[id] {
			t.Errorf("schedule targets %q, which is not a demo environment", id)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM incidents WHERE narrative_status != 'none'"); n != 0 {
		t.Errorf("%d incidents ask for a narration", n)
	}
	if n := count(t, conn, "SELECT COUNT(*) FROM users WHERE password_hash != ''"); n != 1 {
		t.Errorf("login-capable users: got %d, want only the real one", n)
	}
}

func TestHistoryAuditChainVerifies(t *testing.T) {
	conn, _, _ := seededHistory(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	res, err := modaudit.NewService(conn, nil, logger, historyTestSecret).VerifyHashChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.UnsealedRows != 0 || res.HashedRows != res.TotalRows {
		t.Fatalf("chain: verified=%v hashed=%d unsealed=%d total=%d reason=%q", res.Verified, res.HashedRows, res.UnsealedRows, res.TotalRows, res.Reason)
	}
	var prev sql.NullString
	if err := conn.QueryRow("SELECT prev_hash FROM audit_logs WHERE id = 'boot-row'").Scan(&prev); err != nil {
		t.Fatal(err)
	}
	if !prev.Valid || prev.String == "" {
		t.Error("the row this boot wrote was not re-linked behind the seeded history")
	}
}

func TestHistorySecondRunInsertsNothing(t *testing.T) {
	conn, envIDs, now := seededHistory(t)
	tables := []string{"audit_logs", "workflows", "workflow_runs", "schedules", "schedule_executions", "alerts", "alert_events", "incidents", "users", "groups", "group_members"}
	before := make(map[string]int, len(tables))
	for _, tb := range tables {
		before[tb] = count(t, conn, "SELECT COUNT(*) FROM "+tb)
	}
	e, _ := historyEstate(now)
	if err := seedHistory(context.Background(), conn, e, envIDs, now.Add(time.Hour), historyTestSecret); err != nil {
		t.Fatal(err)
	}
	for _, tb := range tables {
		if n := count(t, conn, "SELECT COUNT(*) FROM "+tb); n != before[tb] {
			t.Errorf("%s: %d rows after the second run, %d before", tb, n, before[tb])
		}
	}
}

func TestHistoryCronNext(t *testing.T) {
	from := time.Date(2026, 9, 27, 16, 10, 0, 0, time.UTC) // a Sunday
	cases := map[string]string{
		"0 3 * * *":    "2026-09-28T03:00:00Z",
		"30 7 * * 1-5": "2026-09-28T07:30:00Z",
		"0 */6 * * *":  "2026-09-27T18:00:00Z",
		"15 4 1 * *":   "2026-10-01T04:15:00Z",
		"0 22 * * 5":   "2026-10-02T22:00:00Z",
	}
	for spec, want := range cases {
		got, err := cronNext(spec, from)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if rfc(got) != want {
			t.Errorf("%s: got %s, want %s", spec, rfc(got), want)
		}
	}
	for _, p := range schedulePlans {
		if _, err := cronNext(p.cron, from); err != nil {
			t.Errorf("plan %q: %v", p.name, err)
		}
	}
}
