// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	sqlite "modernc.org/sqlite"
)

type fileConnector struct{ dsn string }

func (c fileConnector) Connect(context.Context) (driver.Conn, error) {
	return (&sqlite.Driver{}).Open(c.dsn)
}
func (c fileConnector) Driver() driver.Driver { return &sqlite.Driver{} }

func histogramCount(t *testing.T, backend, operation string) uint64 {
	t.Helper()
	families, err := Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "mcharbor_db_query_duration_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["backend"] == backend && labels["operation"] == operation {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func TestWrappedDatabaseTimesEveryStatementPath(t *testing.T) {
	database := sql.OpenDB(WrapDBConnector(fileConnector{dsn: "file:" + t.TempDir() + "/w.db"}, "sqlite-test"))
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})
	ctx := context.Background()

	execBefore, queryBefore := histogramCount(t, "sqlite-test", "exec"), histogramCount(t, "sqlite-test", "query")

	if _, err := database.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "INSERT INTO t (name) VALUES (?)", "a"); err != nil {
		t.Fatal(err)
	}
	stmt, err := database.PrepareContext(ctx, "INSERT INTO t (name) VALUES (?)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.ExecContext(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			t.Errorf("closing statement: %v", err)
		}
	}()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO t (name) VALUES (?)", "c"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM t WHERE name <> ?", "").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("rows = %d, want 3; the wrapper changed what the driver did", count)
	}

	if got := histogramCount(t, "sqlite-test", "exec") - execBefore; got != 4 {
		t.Fatalf("exec observations = %d, want 4 (create, insert, prepared insert, tx insert)", got)
	}
	if got := histogramCount(t, "sqlite-test", "query") - queryBefore; got != 1 {
		t.Fatalf("query observations = %d, want 1", got)
	}

	errBefore := testutil.ToFloat64(dbErrors.WithLabelValues("sqlite-test", "exec"))
	if _, err := database.ExecContext(ctx, "INSERT INTO missing_table VALUES (1)"); err == nil {
		t.Fatal("expected an error from a missing table")
	}
	if got := testutil.ToFloat64(dbErrors.WithLabelValues("sqlite-test", "exec")) - errBefore; got != 1 {
		t.Fatalf("errors = %v, want 1", got)
	}
}

// TestDialectHooks guards the Dialect contract: Args sees the statement as written
// (not the rewrite) on both the direct and the prepared path, Value maps every read
// value, and Retry re-runs a failed statement exactly once.
func TestDialectHooks(t *testing.T) {
	var seen []string
	retries := 0
	d := Dialect{
		Rewrite: func(q string) string { return strings.ReplaceAll(q, "tbl_app", "t") },
		Args:    func(q string, _ []driver.NamedValue) { seen = append(seen, q) },
		Value: func(v driver.Value) driver.Value {
			if n, ok := v.(int64); ok {
				return n * 10
			}
			return v
		},
		Retry: func(err error) bool { retries++; return true },
	}
	database := sql.OpenDB(WrapDBConnectorWithDialect(fileConnector{dsn: "file:" + t.TempDir() + "/d.db"}, "sqlite-dialect", d))
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, "CREATE TABLE t (n INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "INSERT INTO tbl_app (n) VALUES (?)", 4); err != nil {
		t.Fatal(err)
	}
	stmt, err := database.PrepareContext(ctx, "SELECT n FROM tbl_app WHERE n = ?")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			t.Errorf("closing statement: %v", err)
		}
	}()
	var n int64
	if err := stmt.QueryRowContext(ctx, 4).Scan(&n); err != nil || n != 40 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	for _, q := range seen[1:] {
		if !strings.Contains(q, "tbl_app") {
			t.Fatalf("Args saw the rewritten statement %q", q)
		}
	}
	before := len(seen)
	if _, err := database.ExecContext(ctx, "INSERT INTO missing (n) VALUES (?)", 1); err == nil {
		t.Fatal("expected an error")
	}
	if retries != 1 || len(seen)-before != 2 {
		t.Fatalf("retries = %d, binds = %d; want one retry and a re-bind", retries, len(seen)-before)
	}
}

// TestNodeLabelOnEveryScrapedSeries guards the node identity label: once set, every
// series in the exposition carries node_id with the one constant value.
func TestNodeLabelOnEveryScrapedSeries(t *testing.T) {
	SetNodeID("node-7")
	t.Cleanup(func() { SetNodeID("") })
	families, err := NodeGatherer(Registry).Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, f := range families {
		for _, m := range f.GetMetric() {
			seen++
			found := false
			for _, l := range m.GetLabel() {
				if l.GetName() == "node_id" {
					found = l.GetValue() == "node-7"
				}
			}
			if !found {
				t.Fatalf("%s has no node_id=node-7 label", f.GetName())
			}
		}
	}
	if seen == 0 {
		t.Fatal("no series gathered")
	}
}
