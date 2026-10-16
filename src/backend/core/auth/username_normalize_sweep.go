// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"fmt"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"log/slog"
	"time"
)

// NormalizeUsernamesSweep re-normalises every username in the users
// table to its canonical form (NormalizeUsername) on every boot,
// AFTER migrations have applied.
//
// Why this exists in Go, in addition to migration 132's SQL UPDATE:
// SQLite's lower() is ASCII-only, while NormalizeUsername uses Go's
// strings.ToLower, which is Unicode-aware. A username containing a
// non-ASCII uppercase letter (e.g. "Ärend") survives migration 132's
// UPDATE unchanged, but every login/lockout lookup folds the typed
// username through NormalizeUsername before querying — so that
// account would become permanently unreachable at login without
// this sweep. Running it on every boot (not just once) keeps that
// guarantee even if a row is ever reintroduced with un-normalised
// casing by something outside the application's own write paths
// (an out-of-band SQL import, a restored backup, etc.).
//
// Cheap on the common path: it is a no-op — no rows scanned into an
// update, nothing logged — once every username is already
// canonical, since the comparison happens in Go before any write is
// attempted.
//
// Fails loudly on a UNIQUE-constraint violation: if two existing
// rows fold to the same canonical username (a genuine Unicode
// collision), the sweep aborts and returns the error so the caller
// can fail startup. Silently merging two real accounts on a Unicode
// fold would be a privilege-escalation bug, not a fix — the same
// rule migration 132 already applies to the ASCII case.
func NormalizeUsernamesSweep(db *sql.DB) error {
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning username normalisation sweep: %w", err)
	}
	defer coredb.RollbackTx(tx)

	// No LIMIT: a one-off normalisation sweep over every account. A cap would
	// leave the accounts past it un-normalised with no second pass to catch them.
	rows, err := tx.QueryContext(ctx, "SELECT id, username FROM users")
	if err != nil {
		return fmt.Errorf("reading usernames for normalisation sweep: %w", err)
	}

	type pendingUpdate struct {
		id         string
		normalized string
	}
	var updates []pendingUpdate
	for rows.Next() {
		var id, username string
		if err := rows.Scan(&id, &username); err != nil {
			coredb.CloseRows(rows)
			return fmt.Errorf("scanning username for normalisation sweep: %w", err)
		}
		if normalized := NormalizeUsername(username); normalized != username {
			updates = append(updates, pendingUpdate{id: id, normalized: normalized})
		}
	}
	if err := rows.Err(); err != nil {
		coredb.CloseRows(rows)
		return fmt.Errorf("iterating usernames for normalisation sweep: %w", err)
	}
	coredb.CloseRows(rows)

	if len(updates) == 0 {
		// Common path: already fully normalised. Roll back the
		// read-only transaction and stay silent.
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx,
			"UPDATE users SET username = ?, updated_at = ? WHERE id = ?",
			u.normalized, now, u.id,
		); err != nil {
			return fmt.Errorf("username normalisation sweep: folding username for user id %s to %q would collide with an existing account: %w", u.id, u.normalized, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing username normalisation sweep: %w", err)
	}

	slog.Info("username normalisation sweep updated rows", "count", len(updates))
	return nil
}
