// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	mdb "github.com/therealmcsparrow/mcharbor/core/db"
)

// reconcileInterval throttles the live presence probe. Every probe costs one
// container listing per environment holding an installation, and over the
// agent transport that is a round trip to a remote host — so the store page
// refreshing on a timer must not turn into a listing per render.
const reconcileInterval = 20 * time.Second

// reconcileGrace keeps a freshly recorded installation off the chopping block.
// An install records its row and then starts the stack; a probe that lands in
// between would see no containers yet and prune the record it just wrote.
const reconcileGrace = 2 * time.Minute

// StackPresenceChecker answers "which stacks actually exist in this
// environment right now" for the App Store's installed-flag reconciliation.
//
// Implemented by the stacks service, which discovers compose projects from
// Docker labels and container links. Local, remote and agent environments all
// answer the same way.
type StackPresenceChecker interface {
	// PresentStackNames returns the set of stack names present in envID.
	// A false second return means the environment could not be inspected —
	// which is explicitly *not* the same as "nothing is installed there".
	PresentStackNames(ctx context.Context, envID string) (map[string]bool, bool)
}

// SetStackPresence connects the reconciler to live environment state. Without
// it the installed flag is driven by the installation records alone.
func (s *Service) SetStackPresence(presence StackPresenceChecker) {
	s.presence = presence
}

type installationRow struct {
	id          string
	stackName   string
	envID       string
	installedAt string
}

// reconcileInstallations drops installation records whose app no longer exists
// in its environment, so the "Installed" flag tracks reality.
//
// The rules, in order of who owns the record:
//
//   - The store installs the app: the record is written and the flag is on. It
//     stays on for as long as the app's containers exist, whatever happens to
//     the stack row (migration 104 removed the FK cascade that used to take
//     the record down with it).
//   - The store uninstalls the app, or the stack is deleted by hand: the
//     record is deleted at the point of removal, by the store and by the
//     stacks/containers modules respectively.
//   - The app's containers are removed by hand from the Docker overview: no
//     module knows to touch the record, so it is reconciled away here.
//
// An environment that cannot be inspected yields no verdict — its records are
// left exactly as they are. Losing the flag because an agent was briefly
// offline is the failure this whole path exists to prevent.
//
// Errors are logged, never returned: reconciliation is bookkeeping on a read
// path, and a store listing must still render when Docker is having a bad day.
func (s *Service) reconcileInstallations(ctx context.Context) {
	if s.presence == nil {
		return
	}
	if !s.reconcileDue() {
		return
	}

	rows, err := s.loadInstallationRows(ctx)
	if err != nil {
		s.logger.Warn("appstore: failed to load installations for reconciliation", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	cutoff := time.Now().Add(-reconcileGrace)
	presentByEnv := make(map[string]map[string]bool)
	unknownEnvs := make(map[string]bool)

	// bounded by the installed rows past the grace window; each prune is independent and retried on the next pass
	for _, row := range rows {
		if installedAfter(row.installedAt, cutoff) {
			continue
		}
		if unknownEnvs[row.envID] {
			continue
		}
		present, cached := presentByEnv[row.envID]
		if !cached {
			names, known := s.presence.PresentStackNames(ctx, row.envID)
			if !known {
				unknownEnvs[row.envID] = true
				continue
			}
			present = names
			presentByEnv[row.envID] = names
		}
		if present[row.stackName] {
			continue
		}

		result, err := s.db.ExecContext(ctx, "DELETE FROM appstore_installed WHERE id = ?", row.id)
		if err != nil {
			s.logger.Warn("appstore: failed to prune installation record", "installation", row.id, "stack", row.stackName, "error", err)
			continue
		}
		if mdb.RowsAffected(result) > 0 {
			s.logger.Info("appstore: app no longer present, installed flag cleared",
				"installation", row.id, "stack", row.stackName, "env", row.envID)
		}
	}
}

// reconcileDue reports whether the throttle window has elapsed, and claims the
// window for the caller when it has.
func (s *Service) reconcileDue() bool {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	if time.Since(s.lastReconcile) < reconcileInterval {
		return false
	}
	s.lastReconcile = time.Now()
	return true
}

func (s *Service) loadInstallationRows(ctx context.Context) ([]installationRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stack_name, environment_id, installed_at
		FROM appstore_installed
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying installations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("appstore: closing installation rows failed", "error", err)
		}
	}()

	var out []installationRow
	for rows.Next() {
		var row installationRow
		var envID sql.NullString
		if err := rows.Scan(&row.id, &row.stackName, &envID, &row.installedAt); err != nil {
			return nil, fmt.Errorf("scanning installation: %w", err)
		}
		if envID.Valid {
			row.envID = envID.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// installedAfter reports whether an installation is younger than the cutoff.
// A timestamp that cannot be parsed is treated as old: an unreadable value is
// no reason to keep a record for an app that is demonstrably gone.
func installedAfter(installedAt string, cutoff time.Time) bool {
	ts, err := time.Parse(time.RFC3339, installedAt)
	if err != nil {
		return false
	}
	return ts.After(cutoff)
}
