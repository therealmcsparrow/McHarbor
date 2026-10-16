// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"context"
	"database/sql"
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

const (
	// envPGCopySrc names the SQLite copy to load into Postgres.
	envPGCopySrc = "MCHARBOR_FLEET_PGCOPY_SRC"
	// envPGCopyDst is the postgres:// DSN of an empty database to fill.
	envPGCopyDst = "MCHARBOR_FLEET_PGCOPY_DST"
)

// TestFleetLoadPostgresCopy fills an empty Postgres database from a SQLite
// copy through coredb.CopyAll — the path Settings → System → Database uses —
// so the Postgres plans are read against the schema the product itself produces.
func TestFleetLoadPostgresCopy(t *testing.T) {
	if os.Getenv(envMeasure) != "1" {
		t.Skip("set MCHARBOR_FLEET_MEASURE=1 to run the fleet measurement")
	}
	src := strings.TrimSpace(os.Getenv(envPGCopySrc))
	dst := strings.TrimSpace(os.Getenv(envPGCopyDst))
	if src == "" || dst == "" {
		t.Skipf("set %s and %s to load a Postgres copy", envPGCopySrc, envPGCopyDst)
	}
	if os.Getenv(envTargetIsCopy) != "1" {
		t.Fatalf("%s is not 1: the source must be a copy, never a live instance's database", envTargetIsCopy)
	}

	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	refuseIfSQLiteLive(t, abs)
	srcDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatalf("opening source read-only: %v", err)
	}
	defer func() { closeutil.Report(srcDB.Close(), "srcDB") }()
	srcDB.SetMaxOpenConns(1)

	dstDB, err := coredb.Open(coredb.Config{Driver: coredb.DriverPostgres, DSN: dst})
	if err != nil {
		t.Fatalf("opening destination: %v", err)
	}
	defer func() { closeutil.Report(dstDB.Close(), "dstDB") }()

	ctx := context.Background()
	// An empty destination is the proof it is a scratch database: Migrate plus
	// CopyAll empties every table first, which would destroy a real one.
	existing, err := coredb.ListTables(ctx, dstDB, coredb.DriverPostgres)
	if err != nil {
		t.Fatalf("listing destination tables: %v", err)
	}
	if len(existing) > 0 {
		t.Fatalf("destination already has %d tables: point %s at an empty scratch database", len(existing), envPGCopyDst)
	}

	start := time.Now()
	if err := coredb.Migrate(dstDB, coredb.DriverPostgres); err != nil {
		t.Fatalf("migrating destination: %v", err)
	}
	t.Logf("migrated in %s", time.Since(start).Round(time.Millisecond))

	start = time.Now()
	report, err := coredb.CopyAll(ctx, srcDB, coredb.DriverSQLite, dstDB, coredb.DriverPostgres, coredb.CopyOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("copying: %v", err)
	}
	t.Logf("copied %d rows across %d tables in %s; skipped tables %v; drifted %v; foreign keys relaxed %v",
		report.TotalRows, len(report.Tables), time.Since(start).Round(time.Millisecond),
		report.SkippedTables, report.Drifted, report.ForeignKeysRelaxed)
	for _, tr := range report.Tables {
		if tr.Rows != tr.SourceRows || len(tr.SkippedColumns) > 0 {
			t.Logf("  %s: %d of %d rows, skipped columns %v", tr.Table, tr.Rows, tr.SourceRows, tr.SkippedColumns)
		}
	}

	// A freshly loaded table has no statistics, so every plan read before this
	// would be a sequential scan for reasons unrelated to the indexes.
	if _, err := dstDB.ExecContext(ctx, "ANALYZE"); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}
}
