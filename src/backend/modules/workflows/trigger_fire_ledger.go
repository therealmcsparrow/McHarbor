// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

const (
	// fireLedgerTTL is how long a fire id stays claimed. It outlives the sender's whole
	// retry window, so a retried fire always meets its first delivery on whichever node
	// leads by then.
	fireLedgerTTL = 10 * time.Minute
	// fireLedgerPruneEvery is how often expired fire ids are deleted.
	fireLedgerPruneEvery = time.Minute
	// fireLedgerTimeout bounds each ledger statement.
	fireLedgerTimeout = 5 * time.Second
)

// fireLedger records evaluated fire ids in the shared database. Being shared is the point:
// a fire the old leader started just before it died is recognised by the new leader when
// the sender retries, and a captured fire replayed to another node starts nothing there.
type fireLedger struct {
	database *sql.DB
	now      func() time.Time
}

func newFireLedger(database *sql.DB) *fireLedger {
	return &fireLedger{database: database, now: time.Now}
}

// claim records fire's id before anything starts and reports whether it is new. The
// UNIQUE index decides a race between two nodes, not a read-then-write.
func (f *fireLedger) claim(ctx context.Context, fire nodelink.TriggerFire) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, fireLedgerTimeout)
	defer cancel()
	now := f.now().UTC()
	nowText := now.Format(time.RFC3339)
	_, err := f.database.ExecContext(ctx, `INSERT INTO workflow_trigger_fires (id, fire_id, origin_node, fire_source,
		action_key, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), fire.FireID, fire.Origin, fire.Source, fire.ActionKey,
		now.Add(fireLedgerTTL).Format(time.RFC3339), nowText, nowText)
	if err == nil {
		return true, nil
	}
	var count int
	if lookupErr := f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_trigger_fires WHERE fire_id = ?`,
		fire.FireID).Scan(&count); lookupErr == nil && count > 0 {
		return false, nil
	}
	return false, fmt.Errorf("workflows: recording fire %s: %w", fire.FireID, err)
}

// forget drops a claim whose evaluation failed before any run started, so the sender's
// retry is evaluated rather than reported as a duplicate.
func (f *fireLedger) forget(ctx context.Context, fireID string) error {
	ctx, cancel := context.WithTimeout(ctx, fireLedgerTimeout)
	defer cancel()
	if _, err := f.database.ExecContext(ctx, `DELETE FROM workflow_trigger_fires WHERE fire_id = ?`, fireID); err != nil {
		return fmt.Errorf("workflows: forgetting fire %s: %w", fireID, err)
	}
	return nil
}

// prune deletes expired fire ids and returns how many went.
func (f *fireLedger) prune(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, fireLedgerTimeout)
	defer cancel()
	res, err := f.database.ExecContext(ctx, `DELETE FROM workflow_trigger_fires WHERE expires_at < ?`,
		f.now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("workflows: pruning fire ids: %w", err)
	}
	return db.RowsAffected(res), nil
}

// runFireLedgerPrune deletes expired fire ids until ctx ends. Every node may prune: the
// delete is idempotent, and leaving it to the leader would stall it across a failover.
func (ts *TriggerService) runFireLedgerPrune(ctx context.Context) {
	defer ts.wg.Done()
	ledger := ts.fireLedger()
	if ledger == nil {
		return
	}
	ticker := time.NewTicker(fireLedgerPruneEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := ledger.prune(ctx); err != nil {
				ts.logger.Warn("workflow trigger: pruning the fire ledger failed", "error", err)
			}
		}
	}
}

// fireLedger returns the shared ledger, or nil when the service has no database.
func (ts *TriggerService) fireLedger() *fireLedger {
	if ts.fires != nil && ts.fires.ledger != nil {
		return ts.fires.ledger
	}
	return nil
}
