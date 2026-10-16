// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// resetadmin is a one-shot CLI that resets the local admin password
// for end-to-end testing of the plugin install/upgrade flow. NOT for
// production use — production should rotate via the admin UI.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func main() {
	dbPath := flag.String("db", "/app/data/mcharbor.db", "path to mcharbor.db")
	username := flag.String("user", "admin", "username to reset")
	newPassword := flag.String("password", "test1234", "new password")
	flag.Parse()

	// Usernames are stored lowercase; normalise the CLI flag so this
	// tool still finds the user regardless of the casing typed here.
	uname := auth.NormalizeUsername(*username)

	fmt.Fprintf(os.Stderr, "resetadmin: db=%s user=%s\n", *dbPath, uname)

	if err := run(context.Background(), *dbPath, uname, *newPassword); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = log.Println
}

func run(ctx context.Context, dbPath, uname, newPassword string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open db failed: %w", err)
	}
	defer func() { closeutil.Report(db.Close(), "db") }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db failed: %w", err)
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash failed: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE username = ?`, hash, uname)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	n := coredb.RowsAffected(res)
	if n == 0 {
		return fmt.Errorf("no user %q found", uname)
	}
	fmt.Printf("reset password for user=%s rows=%d\n", uname, n)
	return nil
}
