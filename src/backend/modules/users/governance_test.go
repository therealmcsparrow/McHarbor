// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

type noopArchiver struct{}

func (noopArchiver) ArchiveOwnedKeysTx(context.Context, *sql.Tx, string) error { return nil }

func openGovernanceDB(t *testing.T) (*sql.DB, *Service) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "u.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash, role, is_active) VALUES ('a1', 'admin1', 'x', 'admin', 1), ('a2', 'admin2', 'x', 'admin', 1), ('u1', 'user1', 'x', 'user', 1), ('u2', 'user2', 'x', 'user', 1)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	svc := NewService(database, rbac.NewService(database), nil)
	svc.SetAPIKeyArchiver(noopArchiver{})
	return database, svc
}

func outcomes(res *BulkResult) map[string]string {
	out := map[string]string{}
	for _, it := range res.Items {
		out[it.UserID] = it.Outcome + ":" + it.Reason
	}
	return out
}

// TestBulkDryRunChangesNothingAndMatchesApply is the preview contract: the
// dry run predicts the apply outcome per user, and writes nothing.
func TestBulkDryRunChangesNothingAndMatchesApply(t *testing.T) {
	database, svc := openGovernanceDB(t)
	ctx := context.Background()
	in := BulkInput{Action: BulkDisable, UserIDs: []string{"a1", "a2", "u1", "ghost", "u1"}, DryRun: true}

	preview, err := svc.Bulk(ctx, in, "u2")
	if err != nil {
		t.Fatal(err)
	}
	got := outcomes(preview)
	want := map[string]string{"a1": "would_apply:", "a2": "skipped:last_admin", "u1": "would_apply:", "ghost": "skipped:not_found"}
	for id, w := range want {
		if got[id] != w {
			t.Fatalf("preview %s = %s, want %s (%+v)", id, got[id], w, got)
		}
	}
	if preview.Summary.Total != 4 {
		t.Fatalf("duplicates must collapse: %+v", preview.Summary)
	}
	var disabled int
	if err := database.QueryRow(`SELECT COUNT(*) FROM users WHERE disabled_at IS NOT NULL`).Scan(&disabled); err != nil || disabled != 0 {
		t.Fatalf("dry run wrote: %d %v", disabled, err)
	}

	in.DryRun = false
	applied, err := svc.Bulk(ctx, in, "u2")
	if err != nil {
		t.Fatal(err)
	}
	for id, w := range want {
		expect := w
		if w == "would_apply:" {
			expect = "applied:"
		}
		if outcomes(applied)[id] != expect {
			t.Fatalf("apply %s = %s, want %s", id, outcomes(applied)[id], expect)
		}
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM users WHERE disabled_at IS NOT NULL`).Scan(&disabled); err != nil || disabled != 2 {
		t.Fatalf("apply should disable exactly two: %d %v", disabled, err)
	}
}

func TestBulkRefusesSelfAndLastAdminOnDelete(t *testing.T) {
	_, svc := openGovernanceDB(t)
	res, err := svc.Bulk(context.Background(), BulkInput{Action: BulkDelete, UserIDs: []string{"a1", "a2"}}, "a1")
	if err != nil {
		t.Fatal(err)
	}
	got := outcomes(res)
	if got["a1"] != "skipped:self" || got["a2"] != "applied:" {
		t.Fatalf("delete outcomes: %+v", got)
	}
	again, err := svc.Bulk(context.Background(), BulkInput{Action: BulkDelete, UserIDs: []string{"a1"}}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if outcomes(again)["a1"] != "skipped:last_admin" {
		t.Fatalf("the last admin must survive a bulk delete: %+v", outcomes(again))
	}
	if _, err := svc.Bulk(context.Background(), BulkInput{Action: "promote", UserIDs: []string{"u1"}}, "a1"); !errors.Is(err, ErrBulkInvalid) {
		t.Fatalf("unknown action: %v", err)
	}
}

func TestLifecycleFlags(t *testing.T) {
	database, svc := openGovernanceDB(t)
	old := time.Now().UTC().AddDate(0, 0, -200).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE users SET last_login = ?, created_at = ? WHERE id = 'u1'`, []any{old, old}},
		{`UPDATE users SET last_login = ?, password_locked_until = ? WHERE id = 'u2'`, []any{recent, future}},
		{`UPDATE users SET disabled_at = ?, last_login = ? WHERE id = 'a2'`, []any{recent, recent}},
		{`UPDATE users SET last_login = ? WHERE id = 'a1'`, []any{recent}},
		{`INSERT INTO user_mfa (user_id, enabled) VALUES ('a1', 1)`, nil},
		{`INSERT INTO break_glass_sessions (id, user_id, reason, started_at, expires_at) VALUES ('s', 'a1', 'outage ten', ?, ?)`, []any{recent, future}},
	} {
		if _, err := database.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed %s: %v", q.sql, err)
		}
	}
	rep, err := svc.Lifecycle(context.Background(), 90, 7)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]LifecycleUser{}
	for _, u := range rep.Users {
		by[u.ID] = u
	}
	if !by["u1"].Inactive || by["u2"].Inactive || !by["u2"].Locked || !by["a2"].Disabled || !by["a1"].MFAEnabled || !by["a1"].RecentlyElevated {
		t.Fatalf("flags: %+v", by)
	}
	if rep.Counts.Inactive != 1 || rep.Counts.Locked != 1 || rep.Counts.Disabled != 1 || rep.Counts.NoMFA != 3 || rep.Counts.RecentlyElevated != 1 {
		t.Fatalf("counts: %+v", rep.Counts)
	}
}

// TestBreakGlassLifecycle covers designation, the reason and window limits, the
// one-open-session rule, and that ending a session withdraws the grant.
func TestBreakGlassLifecycle(t *testing.T) {
	database, svc := openGovernanceDB(t)
	ctx := context.Background()
	checker := rbac.NewService(database)

	if _, err := svc.ActivateBreakGlass(ctx, "u1", ActivateInput{Reason: "datacenter on fire", Minutes: 30}); !errors.Is(err, ErrBreakGlassNotDesignated) {
		t.Fatalf("undesignated activation: %v", err)
	}
	if err := svc.Designate(ctx, "u1", "a1", DesignateInput{MaxMinutes: 60}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateBreakGlass(ctx, "u1", ActivateInput{Reason: "fire", Minutes: 30}); !errors.Is(err, ErrBreakGlassInvalid) {
		t.Fatalf("a short reason must be refused: %v", err)
	}
	if _, err := svc.ActivateBreakGlass(ctx, "u1", ActivateInput{Reason: "datacenter on fire", Minutes: 120}); !errors.Is(err, ErrBreakGlassInvalid) {
		t.Fatalf("a window over the designation must be refused: %v", err)
	}
	if ok, err := checker.HasPermission(ctx, "u1", "", rbac.PermSettingsManage); err != nil || ok {
		t.Fatal("no grant before activation")
	}
	sess, err := svc.ActivateBreakGlass(ctx, "u1", ActivateInput{Reason: "datacenter on fire", Minutes: 30})
	if err != nil || sess == nil {
		t.Fatalf("activate: %v", err)
	}
	if ok, err := checker.HasPermission(ctx, "u1", "", rbac.PermSettingsManage); err != nil || !ok {
		t.Fatal("an open session must grant through the real permission path")
	}
	if _, err := svc.ActivateBreakGlass(ctx, "u1", ActivateInput{Reason: "datacenter on fire", Minutes: 30}); !errors.Is(err, ErrBreakGlassActive) {
		t.Fatalf("second activation: %v", err)
	}
	if _, err := svc.EndBreakGlass(ctx, "u1", "a1"); err != nil {
		t.Fatal(err)
	}
	if ok, err := checker.HasPermission(ctx, "u1", "", rbac.PermSettingsManage); err != nil || ok {
		t.Fatal("an ended session must stop granting")
	}
	if err := svc.Undesignate(ctx, "u1", "a1"); err != nil {
		t.Fatal(err)
	}
	if st, err := svc.BreakGlassStatusFor(ctx, "u1"); err != nil || st.Designated {
		t.Fatal("undesignated account still designated")
	}
}
