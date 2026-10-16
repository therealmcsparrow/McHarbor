// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// envSkewPGDSN names an empty scratch Postgres database the skewed fixture is
// copied into through coredb.CopyAll, so both engines read the same rows.
const envSkewPGDSN = "MCHARBOR_FLEET_SKEW_PG_DSN"

const (
	skewAudit     = 200_000
	skewLifecycle = 300_000
	// skewQuietEnv is the environment that owns one row in a thousand, the
	// shape of the real install's HomeLAB (175 of 191,659 lifecycle rows).
	skewQuietEnv = "env-19"
)

// skewEnv reproduces the real install's shape: 98 % of rows carry no
// environment (NULL, or the empty string older writers stored), one busy
// environment holds most of the rest, and a quiet one holds 0.1 %.
func skewEnv(i int) any {
	switch {
	case i%1000 == 0:
		return skewQuietEnv
	case i%50 == 0:
		return "env-00"
	case i%10 == 1:
		return ""
	default:
		return nil
	}
}

// skewProbes are the two list queries the skewed shape broke, in the form the
// services build them, each with its count query.
func skewProbes() []probe {
	return []probe{
		{"lifecycle", "quiet env + error", `SELECT l.id, le.name FROM lifecycle_events l LEFT JOIN environments le ON le.id = l.environment_id WHERE 1=1 AND l.environment_id = ? AND l.severity = ? ORDER BY l.timestamp DESC LIMIT ? OFFSET ?`, []any{skewQuietEnv, "error", 50, 0}},
		{"lifecycle", "quiet env + error count", `SELECT COUNT(*) FROM lifecycle_events l WHERE 1=1 AND l.environment_id = ? AND l.severity = ?`, []any{skewQuietEnv, "error"}},
		{"lifecycle", "busy env + error", `SELECT l.id, le.name FROM lifecycle_events l LEFT JOIN environments le ON le.id = l.environment_id WHERE 1=1 AND l.environment_id = ? AND l.severity = ? ORDER BY l.timestamp DESC LIMIT ? OFFSET ?`, []any{"env-00", "error", 50, 0}},
		{"audit", "env, OR predicate", `SELECT al.id FROM audit_logs al WHERE (al.environment_id = ? OR al.environment_id IS NULL OR al.environment_id = '') ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, []any{skewQuietEnv, 25, 0}},
		{"audit", "env count, OR predicate", `SELECT COUNT(*) FROM audit_logs al WHERE (al.environment_id = ? OR al.environment_id IS NULL OR al.environment_id = '')`, []any{skewQuietEnv}},
		{"audit", "env, COALESCE IN", `SELECT al.id FROM audit_logs al WHERE COALESCE(al.environment_id, '') IN (?, '') ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, []any{skewQuietEnv, 25, 0}},
		{"audit", "env count, COALESCE IN", `SELECT COUNT(*) FROM audit_logs al WHERE COALESCE(al.environment_id, '') IN (?, '')`, []any{skewQuietEnv}},
		{"audit", "env + action, COALESCE IN", `SELECT al.id FROM audit_logs al WHERE al.action = ? AND COALESCE(al.environment_id, '') IN (?, '') ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, []any{"delete", skewQuietEnv, 25, 0}},
		{"audit", "env, COALESCE IN, page 2000", `SELECT al.id FROM audit_logs al WHERE COALESCE(al.environment_id, '') IN (?, '') ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, []any{skewQuietEnv, 25, 50_000}},
	}
}

// TestFleetMeasureSkewed seeds audit_logs and lifecycle_events with the real
// install's 98 % env-less skew and reports plan and median latency for the two
// list queries that shape broke, before and after statistics, on SQLite and —
// when a scratch DSN is given — on Postgres loaded through coredb.CopyAll.
func TestFleetMeasureSkewed(t *testing.T) {
	if os.Getenv(envMeasure) != "1" {
		t.Skip("set MCHARBOR_FLEET_MEASURE=1 to run the fleet measurement")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "skew.db")
	db, err := coredb.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeutil.Report(db.Close(), "db") }()
	if err := coredb.Migrate(db, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	// OpenSQLite ran PRAGMA optimize on an empty schema; drop what it wrote so
	// the first measurement is the "never analyzed" state the plan describes.
	if _, err := db.Exec("DROP TABLE IF EXISTS sqlite_stat1"); err != nil {
		t.Logf("dropping sqlite_stat1: %v", err)
	}

	start := time.Now()
	seed(t, db, "environments", fleetEnvs, func(i int) map[string]any {
		return map[string]any{"id": env(i), "name": env(i)}
	})
	seed(t, db, "users", 1, func(int) map[string]any { return map[string]any{"id": "u1", "username": "admin"} })
	seed(t, db, "audit_logs", skewAudit, func(i int) map[string]any {
		return map[string]any{"user_id": "u1", "username": "admin", "action": []string{"create", "update", "delete", "login"}[i%4],
			"severity": "info", "entity_type": "network", "entity_id": fmt.Sprintf("n%03d", i%dbContainers),
			"environment_id": skewEnv(i), "timestamp": ago(i, skewAudit, 90*24*time.Hour)}
	})
	seed(t, db, "lifecycle_events", skewLifecycle, func(i int) map[string]any {
		return map[string]any{"environment_id": skewEnv(i), "subject_type": "network", "subject_id": fmt.Sprintf("n%03d", i%dbContainers),
			"subject_name": fmt.Sprintf("net-%d", i%dbContainers), "severity": []string{"info", "warning", "error", "info", "error"}[(i/7)%5],
			"timestamp": ago(i, skewLifecycle, 30*24*time.Hour)}
	})
	t.Logf("seeded skewed fixture in %s", time.Since(start).Round(time.Second))

	sqlite := &target{db: db, driver: coredb.DriverSQLite, label: "skewed fixture", seeded: true}
	dropSkewIndexes(t, db)
	measureSkew(ctx, t, sqlite, "before 120, no statistics")
	createSkewIndexes(t, db)
	measureSkew(ctx, t, sqlite, "after 120, no statistics")
	if err := coredb.Optimize(ctx, db, coredb.DriverSQLite); err != nil {
		t.Fatalf("optimize: %v", err)
	}
	measureSkew(ctx, t, sqlite, "after 120, after PRAGMA optimize=0x10002")

	dsn := strings.TrimSpace(os.Getenv(envSkewPGDSN))
	if dsn == "" {
		t.Logf("%s not set; Postgres half skipped", envSkewPGDSN)
		return
	}
	pg, err := coredb.Open(coredb.Config{Driver: coredb.DriverPostgres, DSN: dsn})
	if err != nil {
		t.Fatalf("opening postgres: %v", err)
	}
	defer func() { closeutil.Report(pg.Close(), "pg") }()
	existing, err := coredb.ListTables(ctx, pg, coredb.DriverPostgres)
	if err != nil {
		t.Fatal(err)
	}
	if len(existing) > 0 {
		t.Fatalf("%s already has %d tables: point it at an empty scratch database", envSkewPGDSN, len(existing))
	}
	if err := coredb.Migrate(pg, coredb.DriverPostgres); err != nil {
		t.Fatalf("migrating postgres: %v", err)
	}
	if _, err := coredb.CopyAll(ctx, db, coredb.DriverSQLite, pg, coredb.DriverPostgres, coredb.CopyOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}); err != nil {
		t.Fatalf("copying the fixture into postgres: %v", err)
	}
	if _, err := pg.ExecContext(ctx, "ANALYZE"); err != nil {
		t.Fatal(err)
	}
	pg.SetMaxOpenConns(1)
	pgTarget := &target{db: pg, driver: coredb.DriverPostgres, label: "skewed fixture", seeded: true}
	measureSkew(ctx, t, pgTarget, "after 120, after ANALYZE")
	dropSkewIndexes(t, pg)
	measureSkew(ctx, t, pgTarget, "before 120, after ANALYZE")
}

// skewIndexes are the two indexes 120_skewed_data_indexes.sql adds, dropped and
// recreated here so one fixture yields both the before and the after plan.
var skewIndexes = []string{"idx_lifecycle_events_env_sev_time", "idx_audit_logs_env_scope_time"}

func dropSkewIndexes(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range skewIndexes {
		if _, err := db.Exec("DROP INDEX IF EXISTS " + name); err != nil {
			t.Fatalf("dropping %s: %v", name, err)
		}
	}
}

func createSkewIndexes(t *testing.T, db *sql.DB) {
	t.Helper()
	text, err := coredb.MigrationSQL("120_skewed_data_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(text); err != nil {
		t.Fatalf("recreating the 120 indexes: %v", err)
	}
}

// measureSkew logs plan and median latency for every skew probe.
func measureSkew(ctx context.Context, t *testing.T, tgt *target, state string) {
	t.Helper()
	var report []string
	for _, p := range skewProbes() {
		qp, err := explainPlan(ctx, tgt, p.query, p.args)
		if err != nil {
			t.Errorf("%s / %s: %v", p.endpoint, p.variant, err)
			continue
		}
		d, n, err := timeQuery(ctx, tgt.db, rebind(tgt.driver, p.query), p.args)
		if err != nil {
			t.Errorf("%s / %s: %v", p.endpoint, p.variant, err)
			continue
		}
		report = append(report, fmt.Sprintf("%8.2f ms  %6d rows  %-10s %-30s %s",
			float64(d.Microseconds())/1000, n, p.endpoint, p.variant, qp))
	}
	t.Log(tgt.label + " (" + string(tgt.driver) + ", " + state + ")\n" + strings.Join(report, "\n"))
}
