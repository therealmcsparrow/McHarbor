// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const (
	fleetEnvs     = 20
	dbContainers  = 300
	tsLayout      = "2006-01-02 15:04:05"
	seedBatchSize = 5000
)

var seedEpoch = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

type column struct {
	name, typ string
	notNull   bool
	hasDflt   bool
	pk        bool
}

func tableColumns(t *testing.T, db *sql.DB, table string) []column {
	t.Helper()
	rows, err := db.Query(`SELECT name, type, "notnull", dflt_value IS NOT NULL, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var cols []column
	for rows.Next() {
		var c column
		var notNull, pk int
		if err := rows.Scan(&c.name, &c.typ, &notNull, &c.hasDflt, &pk); err != nil {
			t.Fatal(err)
		}
		c.notNull, c.pk = notNull == 1, pk > 0
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cols) == 0 {
		t.Fatalf("table %s does not exist", table)
	}
	return cols
}

// seed inserts n rows into table. gen supplies the columns a query filters or
// sorts on; every other NOT NULL column without a default gets a filler.
func seed(t *testing.T, db *sql.DB, table string, n int, gen func(i int) map[string]any) {
	t.Helper()
	cols := tableColumns(t, db, table)
	sample := gen(0)
	var names []string
	for _, c := range cols {
		if _, ok := sample[c.name]; ok || c.pk || (c.notNull && !c.hasDflt) {
			names = append(names, c.name)
		}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	stmtSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(names, ","), placeholders)
	for start := 0; start < n; start += seedBatchSize {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		func() {
			stmt, err := tx.Prepare(stmtSQL)
			if err != nil {
				t.Fatalf("%s: %v", table, err)
			}
			defer func() { closeutil.Report(stmt.Close(), "stmt") }()
			for i := start; i < min(start+seedBatchSize, n); i++ {
				row := gen(i)
				args := make([]any, len(names))
				for j, name := range names {
					if v, ok := row[name]; ok {
						args[j] = v
						continue
					}
					for _, c := range cols {
						if c.name != name {
							continue
						}
						switch {
						case c.pk || name == "id":
							args[j] = fmt.Sprintf("%s-%d", table, i)
						case strings.Contains(strings.ToUpper(c.typ), "INT"), strings.Contains(strings.ToUpper(c.typ), "REAL"):
							args[j] = 0
						case strings.HasSuffix(name, "_at") || name == "timestamp":
							args[j] = seedEpoch.Format(tsLayout)
						default:
							args[j] = fmt.Sprintf("x%d", i)
						}
					}
				}
				if _, err := stmt.Exec(args...); err != nil {
					t.Fatalf("seeding %s row %d: %v", table, i, err)
				}
			}
		}()
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func env(i int) string { return fmt.Sprintf("env-%02d", i%fleetEnvs) }

// ago spreads n rows evenly back over span from the seed epoch.
func ago(i, n int, span time.Duration) string {
	return seedEpoch.Add(-time.Duration(int64(span) / int64(n) * int64(i))).Format(tsLayout)
}

type probe struct {
	endpoint, variant, query string
	args                     []any
}

func timeQuery(ctx context.Context, db *sql.DB, q string, args []any) (time.Duration, int, error) {
	var samples []time.Duration
	rowsSeen := 0
	for range 5 {
		start := time.Now()
		n, err := countRows(ctx, db, q, args)
		if err != nil {
			return 0, 0, err
		}
		samples = append(samples, time.Since(start))
		rowsSeen = n
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2], rowsSeen, nil
}

// TestFleetMeasureDB seeds a fleet-sized history and reports EXPLAIN QUERY PLAN
// plus median latency for every list endpoint's page and count queries.
func TestFleetMeasureDB(t *testing.T) {
	if os.Getenv(envMeasure) != "1" {
		t.Skip("set MCHARBOR_FLEET_MEASURE=1 to run the fleet measurement")
	}
	ctx := context.Background()
	if tgt := openTarget(t); tgt != nil {
		reportShape(t, tgt)
		reportTableSizes(t, tgt)
		for _, set := range realProbeSets(t, tgt) {
			measureProbes(ctx, t, tgt, newestStamp(t, tgt).value, set)
		}
		return
	}

	db, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "fleet.db"))
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

	seedStart := time.Now()
	seed(t, db, "environments", fleetEnvs, func(i int) map[string]any {
		return map[string]any{"id": env(i), "name": env(i)}
	})
	seed(t, db, "users", 1, func(int) map[string]any { return map[string]any{"id": "u1", "username": "admin"} })

	const (
		nEvents    = 500_000
		nAudit     = 200_000
		nLifecycle = 300_000
		nNotif     = 20_000
		nUsage     = 500_000
		nScans     = 30_000
		nBackupLog = 150_000
		nAgentLogs = 200_000
		nAlertEvt  = 100_000
		nRuns      = 100_000
	)
	seed(t, db, "container_events", nEvents, func(i int) map[string]any {
		return map[string]any{"environment_id": env(i), "container_id": fmt.Sprintf("c%03d", i%dbContainers),
			"container_name": fmt.Sprintf("stack-%d-svc", i%dbContainers), "action": []string{"start", "die", "stop", "health_status"}[i%4],
			"event_type": "container", "timestamp": ago(i, nEvents, 30*24*time.Hour)}
	})
	seed(t, db, "audit_logs", nAudit, func(i int) map[string]any {
		return map[string]any{"user_id": "u1", "username": "admin", "action": []string{"create", "update", "delete", "login"}[i%4],
			"severity": "info", "entity_type": "container", "entity_id": fmt.Sprintf("c%03d", i%dbContainers),
			"environment_id": env(i), "timestamp": ago(i, nAudit, 90*24*time.Hour)}
	})
	seed(t, db, "lifecycle_events", nLifecycle, func(i int) map[string]any {
		return map[string]any{"environment_id": env(i), "subject_type": "container", "subject_id": fmt.Sprintf("c%03d", i%dbContainers),
			"subject_name": fmt.Sprintf("stack-%d-svc", i%dbContainers), "severity": []string{"info", "warning", "error"}[i%3],
			"timestamp": ago(i, nLifecycle, 30*24*time.Hour)}
	})
	seed(t, db, "in_app_notifications", nNotif, func(i int) map[string]any {
		return map[string]any{"level": "info", "title": "t", "message": "m", "created_at": ago(i, nNotif, 90*24*time.Hour)}
	})
	seed(t, db, "in_app_notification_reads", nNotif/2, func(i int) map[string]any {
		return map[string]any{"notification_id": fmt.Sprintf("in_app_notifications-%d", i*2), "user_id": "u1", "read_at": ago(i, nNotif/2, 90*24*time.Hour)}
	})
	seed(t, db, "api_keys", 5, func(i int) map[string]any {
		return map[string]any{"id": fmt.Sprintf("key-%d", i), "name": fmt.Sprintf("k%d", i)}
	})
	seed(t, db, "api_key_usage_events", nUsage, func(i int) map[string]any {
		return map[string]any{"api_key_id": fmt.Sprintf("key-%d", i%5), "occurred_at": ago(i, nUsage, 30*24*time.Hour),
			"method": "GET", "status_code": []int{200, 200, 200, 404, 500}[i%5], "environment_id": env(i)}
	})
	seed(t, db, "scans", nScans, func(i int) map[string]any {
		return map[string]any{"image_ref": fmt.Sprintf("registry/app-%d:1", i%500), "scanner": "trivy", "status": "completed",
			"environment_id": env(i), "created_at": ago(i, nScans, 180*24*time.Hour)}
	})
	seed(t, db, "container_backup_log", nBackupLog, func(i int) map[string]any {
		return map[string]any{"environment_id": env(i), "plan_id": fmt.Sprintf("plan-%d", i%100), "severity": "info",
			"action": "run", "message": "backup step", "created_at": ago(i, nBackupLog, 90*24*time.Hour)}
	})
	seed(t, db, "agent_logs", nAgentLogs, func(i int) map[string]any {
		return map[string]any{"environment_id": env(i), "level": "info", "message": "m", "logged_at": ago(i, nAgentLogs, 7*24*time.Hour)}
	})
	seed(t, db, "host_metrics", fleetEnvs*2880, func(i int) map[string]any {
		return map[string]any{"environment_id": env(i), "timestamp": seedEpoch.Add(-time.Duration(i/fleetEnvs) * 30 * time.Second).Format(tsLayout)}
	})
	seed(t, db, "container_metrics", dbContainers*2880, func(i int) map[string]any {
		c := i % dbContainers
		return map[string]any{"environment_id": env(c), "container_id": fmt.Sprintf("c%03d", c), "container_key": fmt.Sprintf("compose:s/%d", c),
			"container_name": fmt.Sprintf("stack-%d-svc", c), "sampled_at": seedEpoch.Add(-time.Duration(i/dbContainers) * 30 * time.Second).Format(tsLayout)}
	})
	seed(t, db, "alerts", 200, func(i int) map[string]any { return map[string]any{"name": fmt.Sprintf("alert-%03d", i)} })
	seed(t, db, "alert_events", nAlertEvt, func(i int) map[string]any {
		return map[string]any{"alert_id": fmt.Sprintf("alerts-%d", i%200), "state": "firing", "severity": "warning", "created_at": ago(i, nAlertEvt, 90*24*time.Hour)}
	})
	seed(t, db, "workflows", 50, func(i int) map[string]any { return map[string]any{"name": fmt.Sprintf("wf-%d", i)} })
	seed(t, db, "workflow_runs", nRuns, func(i int) map[string]any {
		return map[string]any{"workflow_id": fmt.Sprintf("workflows-%d", i%50), "status": "success", "started_at": ago(i, nRuns, 90*24*time.Hour)}
	})
	if _, err := db.Exec("ANALYZE"); err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded in %s", time.Since(seedStart).Round(time.Second))

	fixture := &target{db: db, driver: coredb.DriverSQLite, label: "synthetic fixture", seeded: true}
	measureProbes(ctx, t, fixture, seedEpoch.Format(tsLayout), probeSet{label: "fixture values"})
}

// fleetProbes are the page and count queries every paginated list endpoint
// issues, in the form its service builds them. anchor is the newest timestamp
// the database holds, so the relative windows land on data that exists.
func fleetProbes(anchor string) []probe {
	at, err := time.Parse(tsLayout, anchor)
	if err != nil {
		at = seedEpoch
	}
	day := at.Add(-24 * time.Hour).Format(tsLayout)
	week := at.Add(-7 * 24 * time.Hour).Format(tsLayout)
	now := at.Add(time.Minute).Format(tsLayout)
	page := []any{25, 0}
	deep := []any{25, 400_000}
	probes := []probe{
		{"activity", "no filter", `SELECT ce.id, env.name FROM container_events ce LEFT JOIN environments env ON env.id = ce.environment_id WHERE 1=1 ORDER BY ce.timestamp DESC LIMIT ? OFFSET ?`, page},
		{"activity", "no filter, page 16000", `SELECT ce.id, env.name FROM container_events ce LEFT JOIN environments env ON env.id = ce.environment_id WHERE 1=1 ORDER BY ce.timestamp DESC LIMIT ? OFFSET ?`, deep},
		{"activity", "count", `SELECT COUNT(*) FROM container_events ce LEFT JOIN environments env ON env.id = ce.environment_id WHERE 1=1`, nil},
		{"activity", "env", `SELECT ce.id FROM container_events ce LEFT JOIN environments env ON env.id = ce.environment_id WHERE ce.environment_id = ? ORDER BY ce.timestamp DESC LIMIT ? OFFSET ?`, []any{"env-03", 25, 0}},
		{"activity", "env count", `SELECT COUNT(*) FROM container_events ce LEFT JOIN environments env ON env.id = ce.environment_id WHERE ce.environment_id = ?`, []any{"env-03"}},
		{"activity", "env + search", `SELECT ce.id FROM container_events ce LEFT JOIN environments env ON env.id = ce.environment_id WHERE ce.environment_id = ? AND (LOWER(ce.container_name) LIKE ? OR LOWER(ce.action) LIKE ? OR LOWER(env.name) LIKE ?) ORDER BY ce.timestamp DESC LIMIT ? OFFSET ?`, []any{"env-03", "%svc%", "%svc%", "%svc%", 25, 0}},
		{"audit", "no filter", `SELECT al.id FROM audit_logs al ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, page},
		{"audit", "count", `SELECT COUNT(*) FROM audit_logs al`, nil},
		{"audit", "env", `SELECT al.id FROM audit_logs al WHERE COALESCE(al.environment_id, '') IN (?, '') ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, []any{"env-03", 25, 0}},
		{"audit", "env count", `SELECT COUNT(*) FROM audit_logs al WHERE COALESCE(al.environment_id, '') IN (?, '')`, []any{"env-03"}},
		{"audit", "action + 7d", `SELECT al.id FROM audit_logs al WHERE al.action = ? AND al.timestamp >= ? AND al.timestamp <= ? ORDER BY al.timestamp DESC LIMIT ? OFFSET ?`, []any{"delete", week, now, 25, 0}},
		{"lifecycle", "no filter", `SELECT l.id, le.name FROM lifecycle_events l LEFT JOIN environments le ON le.id = l.environment_id WHERE 1=1 ORDER BY l.timestamp DESC LIMIT ? OFFSET ?`, []any{50, 0}},
		{"lifecycle", "count", `SELECT COUNT(*) FROM lifecycle_events l WHERE 1=1`, nil},
		{"lifecycle", "env + severity", `SELECT l.id FROM lifecycle_events l WHERE 1=1 AND l.environment_id = ? AND l.severity = ? ORDER BY l.timestamp DESC LIMIT ? OFFSET ?`, []any{"env-03", "error", 50, 0}},
		{"lifecycle", "recent", `SELECT id FROM lifecycle_events ORDER BY timestamp DESC LIMIT ?`, []any{25}},
		{"in-app notifications", "page", `SELECT n.id, r.read_at FROM in_app_notifications n LEFT JOIN in_app_notification_reads r ON r.notification_id = n.id AND r.user_id = ? WHERE r.deleted_at IS NULL ORDER BY CASE WHEN r.read_at IS NULL THEN 0 ELSE 1 END ASC, n.created_at DESC LIMIT ? OFFSET ?`, []any{"u1", 25, 0}},
		{"in-app notifications", "count", `SELECT COUNT(*) FROM in_app_notifications n LEFT JOIN in_app_notification_reads r ON r.notification_id = n.id AND r.user_id = ? WHERE r.deleted_at IS NULL`, []any{"u1"}},
		{"api key usage", "key", `SELECT id FROM api_key_usage_events WHERE api_key_id = ? ORDER BY occurred_at DESC LIMIT ? OFFSET ?`, []any{"key-1", 25, 0}},
		{"api key usage", "key count", `SELECT COUNT(*) FROM api_key_usage_events WHERE api_key_id = ?`, []any{"key-1"}},
		{"api key usage", "key + 5xx + 24h", `SELECT id FROM api_key_usage_events WHERE api_key_id = ? AND occurred_at >= ? AND occurred_at < ? AND status_code >= ? AND status_code < ? ORDER BY occurred_at DESC LIMIT ? OFFSET ?`, []any{"key-1", day, now, 500, 600, 25, 0}},
		{"scans", "no filter", `SELECT sc.id, se.name FROM scans sc LEFT JOIN environments se ON se.id = sc.environment_id WHERE 1=1 ORDER BY sc.created_at DESC LIMIT ? OFFSET ?`, page},
		{"scans", "env", `SELECT sc.id FROM scans sc WHERE 1=1 AND sc.environment_id = ? ORDER BY sc.created_at DESC LIMIT ? OFFSET ?`, []any{"env-03", 25, 0}},
		{"backup logs", "env", `SELECT l.id, be.name FROM container_backup_log l LEFT JOIN environments be ON be.id = l.environment_id WHERE 1=1 AND l.environment_id = ? ORDER BY l.created_at DESC LIMIT ? OFFSET ?`, []any{"env-03", 50, 0}},
		{"backup logs", "env + search", `SELECT l.id FROM container_backup_log l WHERE 1=1 AND l.environment_id = ? AND (l.message LIKE ? OR l.plan_name LIKE ? OR l.container_name LIKE ?) ORDER BY l.created_at DESC LIMIT ? OFFSET ?`, []any{"env-03", "%zzz%", "%zzz%", "%zzz%", 50, 0}},
		{"agent logs", "env", `SELECT id FROM agent_logs WHERE environment_id = ? ORDER BY logged_at DESC LIMIT ? OFFSET ?`, []any{"env-03", 25, 0}},
		{"agent logs", "env count", `SELECT COUNT(*) FROM agent_logs WHERE environment_id = ?`, []any{"env-03"}},
		{"host history", "raw 24h", `SELECT timestamp FROM host_metrics WHERE environment_id = ? AND timestamp >= ? AND timestamp < ? ORDER BY timestamp ASC LIMIT ?`, []any{"env-03", day, now, 5000}},
		{"container history", "raw 24h", `SELECT sampled_at FROM container_metrics WHERE container_key = ? AND environment_id = ? AND sampled_at >= ? AND sampled_at < ? ORDER BY sampled_at ASC LIMIT ?`, []any{"compose:s/3", "env-03", day, now, 5000}},
		{"container history", "identity scope", `SELECT container_key, container_name FROM (SELECT container_key, container_name, sampled_at AS seen FROM container_metrics WHERE environment_id = ? AND container_id = ? UNION ALL SELECT container_key, container_name, bucket_start AS seen FROM container_metrics_rollup WHERE environment_id = ? AND container_id = ?) "rows" ORDER BY seen DESC LIMIT 1`, []any{"env-03", "c003", "env-03", "c003"}},
		{"alerts", "page", `SELECT id FROM alerts ORDER BY name ASC LIMIT ? OFFSET ?`, page},
		{"alert events", "no filter", `SELECT id FROM alert_events ORDER BY created_at DESC LIMIT ? OFFSET ?`, page},
		{"alert events", "severity", `SELECT id FROM alert_events WHERE severity = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, []any{"critical", 25, 0}},
		{"workflow runs", "no filter", `SELECT id FROM workflow_runs ORDER BY started_at DESC LIMIT ? OFFSET ?`, page},
		{"workflow runs", "workflow", `SELECT id FROM workflow_runs WHERE workflow_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`, []any{"workflows-7", 25, 0}},
	}
	return probes
}

// measureProbes reports the chosen plan and the median of five runs for every
// probe. It only reads, so it is safe against a copy of a real database.
func measureProbes(ctx context.Context, t *testing.T, tgt *target, anchor string, set probeSet) {
	t.Helper()
	var report []string
	for _, p := range fleetProbes(anchor) {
		p.args = bindArgs(p.args, set.subst)
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
		report = append(report, fmt.Sprintf("%8.1f ms  %6d rows  %-22s %-24s %-16s %s",
			float64(d.Microseconds())/1000, n, p.endpoint, p.variant,
			strings.Join(scanFlags(tgt.driver, qp), ","), qp))
	}
	t.Log(tgt.label + " (" + string(tgt.driver) + ", " + set.label + ")\n" + strings.Join(report, "\n"))
}

func countRows(ctx context.Context, db *sql.DB, q string, args []any) (int, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	n := 0
	for rows.Next() {
		n++
	}
	return n, rows.Err()
}
