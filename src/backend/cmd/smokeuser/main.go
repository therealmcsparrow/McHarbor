// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// smokeuser is a one-shot CLI that inserts a throwaway admin account for smoke tests. NOT for production use.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	dbPath := "/app/data/mcharbor.db"
	username := auth.NormalizeUsername("smoketest")
	password := "testpass123"

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { closeutil.Report(db.Close(), "db") }()

	id := uuid.NewString()
	// Plain INSERT (not OR REPLACE): under the case-insensitive unique
	// index, OR REPLACE against an existing case-variant username would
	// silently DELETE the real account instead of erroring. Fail loudly
	// on a collision instead.
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role, auth_provider, is_active, display_name) VALUES (?, ?, ?, 'admin', 'local', 1, 'Smoke')`,
		id, username, hash)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	fmt.Printf("OK id=%s username=%s password=%s\n", id, username, password)
	return nil
}
