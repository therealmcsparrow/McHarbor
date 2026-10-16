// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// testmirror exercises the MirrorManifest code path end-to-end:
// it wipes the plugin_<slug>_* contribution rows (the same way
// ClearAll does on uninstall), then calls MirrorManifest and
// confirms the rows reappear. Used to verify the
// uninstall→install-then-sidebar regression fix without needing
// admin auth. NOT shipped in the production image.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"github.com/therealmcsparrow/mcharbor/modules/appstore/plugins"
)

func main() {
	dbPath := flag.String("db", "/app/data/mcharbor.db", "path to mcharbor.db")
	dataDir := flag.String("data", "/app/data", "plugin data dir")
	slug := flag.String("slug", "podman", "plugin slug")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "testmirror: db=%s data=%s slug=%s\n", *dbPath, *dataDir, *slug)

	if err := run(context.Background(), *dbPath, *dataDir, *slug); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
}

func countMenuRows(ctx context.Context, db *sql.DB, slug string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_menu WHERE origin = ?`, slug).Scan(&n); err != nil {
		return 0, fmt.Errorf("count plugin_menu: %w", err)
	}
	return n, nil
}

func run(ctx context.Context, dbPath, dataDir, slug string) error {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { closeutil.Report(db.Close(), "db") }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	svc := plugins.NewService(db, nil, "")

	version, err := svc.LookupVersion(ctx, slug)
	if err != nil {
		return fmt.Errorf("LookupVersion: %w", err)
	}
	if version == "" {
		return fmt.Errorf("plugin %q not installed; aborting", slug)
	}
	fmt.Fprintf(os.Stderr, "found installed version=%s\n", version)

	// Step 1: confirm the boot loader left the menu rows in place.
	beforeCount, err := countMenuRows(ctx, db, slug)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "step 1: plugin_menu rows before wipe = %d\n", beforeCount)

	// Step 2: wipe plugin_<slug> rows (simulate uninstall's ClearAll).
	for _, table := range []string{
		"plugin_routes", "plugin_menu", "plugin_topbar",
		"plugin_context_menus", "plugin_tabs", "plugin_widgets", "plugin_nodes",
	} {
		res, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE origin = ?`, slug) // SQL identifier: table comes from the fixed list above
		if err != nil {
			return fmt.Errorf("wipe %s: %w", table, err)
		}
		n := coredb.RowsAffected(res)
		fmt.Fprintf(os.Stderr, "step 2: deleted %d rows from %s\n", n, table)
	}

	// Step 3: invoke MirrorManifest (the same code path Install +
	// Enable call when the .so is already in memory).
	if err := svc.MirrorManifest(ctx, dataDir, slug, version); err != nil { //nolint:staticcheck // MirrorManifest is a stub that always errors today; the check must stay for when it is implemented
		return fmt.Errorf("MirrorManifest: %w", err)
	}

	// Step 4: verify the rows came back without a container restart.
	afterCount, err := countMenuRows(ctx, db, slug)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "step 4: plugin_menu rows after mirror = %d\n", afterCount)

	if afterCount < 2 {
		return fmt.Errorf("expected at least 2 menu rows after mirror (pods + quadlets), got %d", afterCount)
	}

	// Print the actual rows so we can see them on the console.
	rows, err := db.QueryContext(ctx, `SELECT parent, label_key, path FROM plugin_menu WHERE origin = ? ORDER BY label_key`, slug)
	if err != nil {
		return fmt.Errorf("select: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	fmt.Println("OK — podman menu rows after MirrorManifest:")
	for rows.Next() {
		var parent, key, path string
		if err := rows.Scan(&parent, &key, &path); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		fmt.Printf("  parent=%s label_key=%s path=%s\n", parent, key, path)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate plugin_menu: %w", err)
	}
	return nil
}
