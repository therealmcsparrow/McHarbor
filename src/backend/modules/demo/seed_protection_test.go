// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// protectionFixture is a demo database after the environments were seeded, the
// way Start leaves it before the protection, runbook and reconciler seeds run.
type protectionFixture struct {
	conn   *sql.DB
	estate *estate.Estate
	envIDs map[string]string
	now    time.Time
}

func newProtectionFixture(t *testing.T, m *coredemo.Marker) *protectionFixture {
	t.Helper()
	f := &protectionFixture{conn: historyDB(t), now: time.Now().UTC().Truncate(time.Second)}
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	f.estate = estate.Build(estate.DefaultSeed, f.now)
	ids, err := seedEnvironments(context.Background(), f.conn, enc, f.estate, m)
	if err != nil {
		t.Fatal(err)
	}
	f.envIDs = ids
	return f
}

func rowCount(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	err := conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n)
	if err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

// TestSeedBackupsRefusesOutsideDemoMode: the only database a seed could be
// writing outside demo mode is the real one, and these seeds write dozens of
// rows into tables an operator owns.
func TestSeedBackupsRefusesOutsideDemoMode(t *testing.T) {
	if _, err := coredemo.Load(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	conn := historyDB(t)
	now := time.Now().UTC()

	for name, err := range map[string]error{
		"backups":    firstErr(seedBackups(context.Background(), conn, estate.Build(estate.DefaultSeed, now), nil, now)),
		"runbooks":   firstErrRunbooks(seedRunbooks(context.Background(), conn, now)),
		"reconciler": firstErrReconciler(seedReconciler(context.Background(), conn, estate.Build(estate.DefaultSeed, now), nil, now)),
	} {
		if !errors.Is(err, errNotDemo) {
			t.Errorf("%s: expected errNotDemo outside demo mode, got %v", name, err)
		}
	}
}

func firstErr(_ backupSeedCounts, err error) error               { return err }
func firstErrRunbooks(_ runbookSeedCounts, err error) error      { return err }
func firstErrReconciler(_ reconcilerSeedCounts, err error) error { return err }

// TestSeedBackupsWritesContainerPlansAndRuns: the container half needs no plugin,
// so it must land on a bare demo schema.
func TestSeedBackupsWritesContainerPlansAndRuns(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)

	got, err := seedBackups(context.Background(), f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContainerPlans == 0 {
		t.Fatal("no container backup plans were written")
	}
	if got.ContainerRuns < got.ContainerPlans {
		t.Errorf("every plan needs a run history: %d plan(s), %d run(s)", got.ContainerPlans, got.ContainerRuns)
	}
	if got.Locations != 1 {
		t.Errorf("expected the offsite location to be added once, got %d", got.Locations)
	}

	// Every plan must point at a container that exists, or the page cannot render
	// the row it lists.
	var orphans int
	err = f.conn.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM container_backup_plans p
		LEFT JOIN environments e ON e.id = p.environment_id
		WHERE e.id IS NULL`).Scan(&orphans)
	if err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Errorf("%d plan(s) reference an environment that does not exist", orphans)
	}

	// The history has to carry a failure and a restore, because those are the two
	// rows whose presentation is easiest to get wrong.
	if n := countWhere(t, f.conn, "container_backup_runs", "status = 'failure'"); n == 0 {
		t.Error("the run history carries no failed run, so the error path is never shown")
	}
	if n := countWhere(t, f.conn, "container_backup_runs", "operation = 'restore'"); n == 0 {
		t.Error("the run history carries no restore")
	}
	// A failed run must carry its message: a failed row with an empty error is the
	// defect table-status-errors exists to prevent.
	if n := countWhere(t, f.conn, "container_backup_runs", "status = 'failure' AND (error IS NULL OR error = '')"); n != 0 {
		t.Errorf("%d failed run(s) carry no error message", n)
	}
	// Each run needs its destination rows, or the multi-destination panel is empty.
	if countWhere(t, f.conn, "container_backup_run_destinations", "1=1") < got.ContainerRuns {
		t.Error("some runs have no destination rows")
	}
}

// TestSeedBackupsIsIdempotent: Start runs on every boot, and re-running the seed
// would overwrite whatever the viewer disabled or deleted.
func TestSeedBackupsIsIdempotent(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)
	ctx := context.Background()

	if _, err := seedBackups(ctx, f.conn, f.estate, f.envIDs, f.now); err != nil {
		t.Fatal(err)
	}
	before := rowCount(t, f.conn, "container_backup_plans")
	runsBefore := rowCount(t, f.conn, "container_backup_runs")

	// The viewer disables a plan and deletes a run.
	if _, err := f.conn.ExecContext(ctx, "UPDATE container_backup_plans SET enabled = 0"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conn.ExecContext(ctx, "DELETE FROM container_backup_runs WHERE rowid IN (SELECT rowid FROM container_backup_runs LIMIT 3)"); err != nil {
		t.Fatal(err)
	}

	second, err := seedBackups(ctx, f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if second.ContainerPlans != 0 {
		t.Errorf("the second seed wrote %d plan(s); it must write none", second.ContainerPlans)
	}
	if got := rowCount(t, f.conn, "container_backup_plans"); got != before {
		t.Errorf("plan count changed on the second boot: %d -> %d", before, got)
	}
	if got := rowCount(t, f.conn, "container_backup_runs"); got != runsBefore-3 {
		t.Errorf("the deleted runs came back: %d, want %d", got, runsBefore-3)
	}
	if n := countWhere(t, f.conn, "container_backup_plans", "enabled = 1"); n != 0 {
		t.Errorf("%d plan(s) were re-enabled; the viewer's edit must survive a restart", n)
	}
}

// TestSeedRunbooksWritesStepsAndRuns: a runbook with no steps renders an empty
// panel, which is indistinguishable from a broken one.
func TestSeedRunbooksWritesStepsAndRuns(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)

	got, err := seedRunbooks(context.Background(), f.conn, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runbooks == 0 {
		t.Fatal("no runbooks were written")
	}
	if got.Steps < got.Runbooks*3 {
		t.Errorf("%d runbook(s) but only %d step(s)", got.Runbooks, got.Steps)
	}
	if got.Runs == 0 {
		t.Error("no runbook runs were written")
	}
	// Every runbook must have at least one step, and the positions must start at 0
	// and not repeat, or the ordered list renders wrong.
	rows, err := f.conn.QueryContext(context.Background(), `
		SELECT runbook_id, COUNT(*), MIN(position), COUNT(DISTINCT position)
		FROM runbook_steps GROUP BY runbook_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()
	seen := 0
	for rows.Next() {
		var id string
		var n, minPos, distinct int
		if err := rows.Scan(&id, &n, &minPos, &distinct); err != nil {
			t.Fatal(err)
		}
		seen++
		if minPos != 0 {
			t.Errorf("runbook %s starts at position %d", id, minPos)
		}
		if distinct != n {
			t.Errorf("runbook %s has repeated positions: %d step(s), %d distinct", id, n, distinct)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != got.Runbooks {
		t.Errorf("%d runbook(s) have steps, expected %d", seen, got.Runbooks)
	}
	// Both outcomes must appear, or the outcome column is a single value.
	for _, outcome := range []string{"completed", "abandoned"} {
		if countWhere(t, f.conn, "runbook_runs", "outcome = '"+outcome+"'") == 0 {
			t.Errorf("no run with outcome %q", outcome)
		}
	}
}

// TestSeedReconcilerWritesDriftWithAReport: a drifted watch whose report is empty
// renders a drift badge over an empty panel.
func TestSeedReconcilerWritesDriftWithAReport(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)

	got, err := seedReconciler(context.Background(), f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Watches == 0 {
		t.Fatal("no desired-state watches were written")
	}
	if got.Drifted == 0 {
		t.Error("nothing is drifted, so the drift presentation is never shown")
	}
	if got.Drifted == got.Watches {
		t.Error("everything is drifted, so the in-sync presentation is never shown")
	}
	if n := countWhere(t, f.conn, "desired_states", "drift_detected = 1 AND (last_report IS NULL OR last_report = '')"); n != 0 {
		t.Errorf("%d drifted watch(es) carry no report", n)
	}
	// All three modes must be represented: observe, propose and enforce read very
	// differently to an operator.
	for _, mode := range []string{"observe", "propose", "enforce"} {
		if countWhere(t, f.conn, "desired_states", "mode = '"+mode+"'") == 0 {
			t.Errorf("no watch in mode %q", mode)
		}
	}
	// An enforced, drifted watch records when it last enforced.
	if n := countWhere(t, f.conn, "desired_states", "mode = 'enforce' AND drift_detected = 1 AND last_enforced_at IS NULL"); n != 0 {
		t.Errorf("%d enforcing watch(es) never recorded an enforcement", n)
	}
}

// TestSeedReconcilerIsIdempotent mirrors the backup case: Start runs every boot.
func TestSeedReconcilerIsIdempotent(t *testing.T) {
	m := activateDemo(t)
	f := newProtectionFixture(t, m)
	ctx := context.Background()

	if _, err := seedReconciler(ctx, f.conn, f.estate, f.envIDs, f.now); err != nil {
		t.Fatal(err)
	}
	before := rowCount(t, f.conn, "desired_states")

	second, err := seedReconciler(ctx, f.conn, f.estate, f.envIDs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if second.Watches != 0 {
		t.Errorf("the second seed wrote %d watch(es); it must write none", second.Watches)
	}
	if got := rowCount(t, f.conn, "desired_states"); got != before {
		t.Errorf("watch count changed on the second boot: %d -> %d", before, got)
	}
}

func countWhere(t *testing.T, conn *sql.DB, table, where string) int {
	t.Helper()
	var n int
	err := conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE "+where).Scan(&n)
	if err != nil {
		t.Fatalf("counting %s where %s: %v", table, where, err)
	}
	return n
}
